package cmd

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/tavp-stack/tavpbox/internal/config"
	"github.com/tavp-stack/tavpbox/internal/proxy"
)

var proxyPort int

var proxyStartCmd = &cobra.Command{
	Use:   "proxy:start",
	Short: "Start the TAVPBox reverse proxy",
	RunE: func(cmd *cobra.Command, args []string) error {
		// Check if already running
		if isProxyRunning() {
			fmt.Println("Proxy is already running")
			return nil
		}

		ensureFirewallRules(proxyPort)
		p := proxy.New(proxyPort)
		saveProxyPID()
		globalCfg, _ := config.LoadGlobal()
		return p.Start(globalCfg.DomainSuffix)
	},
}

var proxyStopCmd = &cobra.Command{
	Use:   "proxy:stop",
	Short: "Stop the TAVPBox reverse proxy",
	RunE: func(cmd *cobra.Command, args []string) error {
		if !isProxyRunning() {
			fmt.Println("Proxy is not running")
			return nil
		}

		killProcessOnPort(80)
		home, _ := os.UserHomeDir()
		os.Remove(filepath.Join(home, ".tavpbox", "proxy.pid"))
		fmt.Println("Proxy stopped")
		return nil
	},
}

var proxyStatusCmd = &cobra.Command{
	Use:   "proxy:status",
	Short: "Show proxy status",
	Run: func(cmd *cobra.Command, args []string) {
		if isProxyRunning() {
			fmt.Printf("Proxy is running on port 80\n")

			// Read routes from file directly
			home, _ := os.UserHomeDir()
			routesFile := filepath.Join(home, ".tavpbox", "proxy", "routes.json")
			data, err := os.ReadFile(routesFile)
			if err == nil {
				var routes []proxy.Route
				if err := json.Unmarshal(data, &routes); err == nil && len(routes) > 0 {
					fmt.Println("\nRoutes:")
					for _, r := range routes {
						fmt.Printf("  %s → %s:%d\n", r.Domain, r.IP, r.Port)
					}
				}
			}
		} else {
			fmt.Println("Proxy is not running")
		}
	},
}

func isProxyRunning() bool {
	conn, err := net.DialTimeout("tcp", "127.0.0.1:80", 1*time.Second)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// ensureFirewallRules adds Windows Firewall inbound rules for TAVPBox ports
// so devices on the same LAN can access the proxy. No-op on non-Windows.
func ensureFirewallRules(port int) {
	if runtime.GOOS != "windows" {
		return
	}

	// Check if rule already exists
	check := exec.Command("netsh", "advfirewall", "firewall", "show", "rule",
		"name=TAVPBox-Proxy-"+fmt.Sprintf("%d", port))
	if check.Run() == nil {
		return // rule already exists
	}

	rules := []struct {
		name string
		port int
	}{
		{"TAVPBox-Proxy-" + fmt.Sprintf("%d", port), port},
		{"TAVPBox-Proxy-443", 443},
	}

	for _, r := range rules {
		args := []string{
			"advfirewall", "firewall", "add", "rule",
			"name=" + r.name,
			"dir=in", "action=allow", "protocol=TCP",
			"localport=" + fmt.Sprintf("%d", r.port),
			"enable=yes",
			"profile=any",
		}
		if out, err := exec.Command("netsh", args...).CombinedOutput(); err != nil {
			fmt.Printf("  ⚠ Firewall rule (%d): %v — %s\n", r.port, err, strings.TrimSpace(string(out)))
		} else {
			fmt.Printf("  ✓ Firewall rule added: %s (port %d)\n", r.name, r.port)
		}
	}
}

// killProcessOnPort kills whatever process is listening on the given port
func killProcessOnPort(port int) {
	if runtime.GOOS == "windows" {
		// Use netstat + taskkill on Windows
		addr := fmt.Sprintf(":%d", port)
		out, err := exec.Command("netstat", "-ano").Output()
		if err != nil {
			return
		}
		for _, line := range strings.Split(string(out), "\n") {
			if strings.Contains(line, addr) && strings.Contains(line, "LISTENING") {
				fields := strings.Fields(line)
				if len(fields) > 4 {
					pid := fields[len(fields)-1]
					if pid != "0" {
						exec.Command("taskkill", "/F", "/PID", pid).Run()
					}
				}
			}
		}
	} else {
		// Use lsof + kill on Linux/macOS
		out, err := exec.Command("lsof", "-ti", fmt.Sprintf(":%d", port)).Output()
		if err == nil {
			for _, pid := range strings.Split(strings.TrimSpace(string(out)), "\n") {
				if pid != "" {
					exec.Command("kill", "-9", pid).Run()
				}
			}
		}
	}
}

func saveProxyPID() {
	home, _ := os.UserHomeDir()
	pidFile := filepath.Join(home, ".tavpbox", "proxy.pid")
	os.MkdirAll(filepath.Dir(pidFile), 0755)
	os.WriteFile(pidFile, []byte(fmt.Sprintf("%d", os.Getpid())), 0644)
}

func ensureProxyRunning() {
	if isProxyRunning() {
		return
	}

	ensureFirewallRules(80)
	execPath, _ := os.Executable()
	cmd := exec.Command(execPath, "proxy:start", "-p", "80")
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.Stdin = nil

	detachProcess(cmd)

	if err := cmd.Start(); err != nil {
		fmt.Printf("  ⚠ Could not start proxy: %v\n", err)
		return
	}

	time.Sleep(500 * time.Millisecond)
	fmt.Println("  Proxy started in background (port 80)")
}

func restartProxy() {
	// Kill whatever is on port 80
	killProcessOnPort(80)

	// Clean up PID file
	home, _ := os.UserHomeDir()
	os.Remove(filepath.Join(home, ".tavpbox", "proxy.pid"))

	// Wait for ports to be free
	time.Sleep(1 * time.Second)

	// Start fresh proxy
	ensureProxyRunning()
}

func init() {
	proxyStartCmd.Flags().IntVarP(&proxyPort, "port", "p", 80, "Port to listen on")
	proxyStopCmd.Flags().IntVarP(&proxyPort, "port", "p", 80, "Port")
	proxyStatusCmd.Flags().IntVarP(&proxyPort, "port", "p", 80, "Port")

	rootCmd.AddCommand(proxyStartCmd, proxyStopCmd, proxyStatusCmd)
}
