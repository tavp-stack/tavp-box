package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/tavp-stack/tavpbox/internal/config"
	"github.com/tavp-stack/tavpbox/internal/podman"
	"github.com/tavp-stack/tavpbox/internal/proxy"
)

var startAll bool

var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the project container (or all with --all)",
	RunE: func(cmd *cobra.Command, args []string) error {
		client := podman.New()

		// Ensure Podman machine is running
		if err := client.EnsureRunning(); err != nil {
			return fmt.Errorf("podman: %w", err)
		}

		if startAll {
			return startAllContainers(client)
		}

		_, cfg, err := config.FindProject()
		if err != nil {
			return err
		}
		cname := client.ContainerName(cfg.Name)

		// Auto-create if container doesn't exist
		exists, _ := client.Exists(cname)
		if !exists {
			fmt.Printf("Container for '%s' not found. Creating...\n", cfg.Name)
			// Redirect to create command
			return createCmd.RunE(cmd, args)
		}

		fmt.Printf("Starting '%s'...\n", cfg.Name)
		if err := client.Start(cname); err != nil {
			return fmt.Errorf("start: %w", err)
		}

		globalCfg, _ := config.LoadGlobal()
		domain := cfg.Name + "." + globalCfg.DomainSuffix

		// Update routes with correct host port
		hostPort := client.GetHostPort(cname, "80")
		if hostPort > 0 {
			p := proxy.New(80)
			p.AddRoute(domain, "127.0.0.1", hostPort)
			addServiceRoutes(p, client, cname, cfg.Name, globalCfg.DomainSuffix)
		}

		// Always restart proxy to ensure routes are fresh
		restartProxy()

		fmt.Printf("✓ %s started\n", cfg.Name)
		fmt.Printf("  URL: http://%s\n", domain)
		return nil
	},
}

func startAllContainers(client *podman.Client) error {
	// Ensure Podman machine is running
	if err := client.EnsureRunning(); err != nil {
		return fmt.Errorf("podman: %w", err)
	}

	fmt.Println("Starting all TAVPBox containers...")

	containers, err := client.List()
	if err != nil {
		return fmt.Errorf("list containers: %w", err)
	}

	if len(containers) == 0 {
		fmt.Println("No containers found. Run: tavpbox create")
		return nil
	}

	globalCfg, _ := config.LoadGlobal()
	p := proxy.New(80)
	started := 0

	for _, c := range containers {
		name := client.StripPrefix(c.Name)
		if err := client.Start(c.Name); err != nil {
			fmt.Printf("  ⚠ %s: %v\n", name, err)
			continue
		}
		domain := name + "." + globalCfg.DomainSuffix
		fmt.Printf("  ✓ %s → http://%s\n", name, domain)
		started++

		// Register route for this container (same as single-project start)
		hostPort := client.GetHostPort(c.Name, "80")
		if hostPort > 0 {
			p.AddRoute(domain, "127.0.0.1", hostPort)
			addServiceRoutes(p, client, c.Name, name, globalCfg.DomainSuffix)
		}
	}

	// Restart proxy to ensure all routes are fresh
	fmt.Println("\nStarting proxy...")
	restartProxy()

	fmt.Printf("\n✓ %d containers started + proxy running\n", started)
	return nil
}

var stopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the project container",
	RunE: func(cmd *cobra.Command, args []string) error {
		_, cfg, err := config.FindProject()
		if err != nil {
			return err
		}
		client := podman.New()
		cname := client.ContainerName(cfg.Name)

		fmt.Printf("Stopping '%s'...\n", cfg.Name)
		if err := client.Stop(cname); err != nil {
			return fmt.Errorf("stop: %w", err)
		}
		fmt.Printf("✓ %s stopped — RAM freed\n", cfg.Name)
		return nil
	},
}

var restartCmd = &cobra.Command{
	Use:   "restart",
	Short: "Restart the project container",
	RunE: func(cmd *cobra.Command, args []string) error {
		_, cfg, err := config.FindProject()
		if err != nil {
			return err
		}
		client := podman.New()
		cname := client.ContainerName(cfg.Name)

		fmt.Printf("Restarting '%s'...\n", cfg.Name)
		if err := client.Restart(cname); err != nil {
			return fmt.Errorf("restart: %w", err)
		}

		globalCfg, _ := config.LoadGlobal()
		domain := cfg.Name + "." + globalCfg.DomainSuffix

		// Update routes with correct host port
		hostPort := client.GetHostPort(cname, "80")
		if hostPort > 0 {
			p := proxy.New(80)
			p.AddRoute(domain, "127.0.0.1", hostPort)
			addServiceRoutes(p, client, cname, cfg.Name, globalCfg.DomainSuffix)
		}

		// Restart proxy to ensure routes are fresh
		restartProxy()

		fmt.Printf("✓ %s restarted\n", cfg.Name)
		fmt.Printf("  URL: http://%s\n", domain)
		return nil
	},
}

// addServiceRoutes re-registers proxy routes for optional services by
// detecting their host port mappings, so start/restart/--all restore the
// same routes create would have added (#25).
func addServiceRoutes(p *proxy.Proxy, client *podman.Client, cname, name, suffix string) {
	if port := client.GetHostPort(cname, "8025"); port > 0 {
		p.AddRoute(name+"-mailpit."+suffix, "127.0.0.1", port)
	}
	if port := client.GetHostPort(cname, "8081"); port > 0 {
		p.AddRoute(name+"-adminer."+suffix, "127.0.0.1", port)
	}
	if port := client.GetHostPort(cname, "8080"); port > 0 {
		p.AddRoute(name+"-phpmyadmin."+suffix, "127.0.0.1", port)
	}
}

func init() {
	startCmd.Flags().BoolVar(&startAll, "all", false, "Start all containers + proxy")
}
