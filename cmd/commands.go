package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tavp-stack/tavpbox/internal/config"
	"github.com/tavp-stack/tavpbox/internal/podman"
	"github.com/tavp-stack/tavpbox/internal/proxy"
)

var sshCmd = &cobra.Command{
	Use:   "ssh [command...]",
	Short: "SSH into the project container or run a command",
	RunE: func(cmd *cobra.Command, args []string) error {
		_, cfg, err := config.FindProject()
		if err != nil {
			return err
		}
		client := podman.New()
		cname := client.ContainerName(cfg.Name)

		if len(args) > 0 {
			output, err := client.Exec(cname, args...)
			if len(output) > 0 {
				fmt.Print(output)
			}
			return err
		}
		return client.ExecInteractive(cname, "bash")
	},
}

var listCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List all TAVPBox containers",
	RunE: func(cmd *cobra.Command, args []string) error {
		client := podman.New()
		containers, err := client.List()
		if err != nil {
			return err
		}

		if len(containers) == 0 {
			fmt.Println("No containers. Run: tavpbox init && tavpbox create")
			return nil
		}

		fmt.Printf("%-25s %-12s %-20s %s\n", "NAME", "STATUS", "IMAGE", "PORTS")
		fmt.Println(strings.Repeat("-", 80))
		for _, c := range containers {
			name := client.StripPrefix(c.Name)
			status := c.Status
			if strings.Contains(status, "Up") {
				status = "running"
			} else {
				status = "stopped"
			}
			fmt.Printf("%-25s %-12s %-20s %s\n", name, status, c.Image, c.Ports)
		}
		return nil
	},
}

var infoCmd = &cobra.Command{
	Use:   "info",
	Short: "Show project container info",
	RunE: func(cmd *cobra.Command, args []string) error {
		_, cfg, err := config.FindProject()
		if err != nil {
			return err
		}
		client := podman.New()
		cname := client.ContainerName(cfg.Name)
		ip, _ := client.GetIP(cname)
		hostPort := client.GetHostPort(cname, "80")
		globalCfg, _ := config.LoadGlobal()

		domain := cfg.Name + "." + globalCfg.DomainSuffix

		fmt.Println(strings.Repeat("=", 50))
		fmt.Printf(" %s\n", cfg.Name)
		fmt.Println(strings.Repeat("=", 50))
		fmt.Printf(" Recipe:    %s\n", cfg.Recipe)
		fmt.Printf(" IP:        %s\n", ip)
		fmt.Printf(" Webroot:   %s\n", cfg.Webroot)
		fmt.Printf(" RAM:       %s\n", cfg.RAM)
		fmt.Printf(" CPU:       %d\n", cfg.CPU)

		// URLs
		fmt.Println("\n URLs:")
		if hostPort > 0 {
			fmt.Printf("   Direct:  http://localhost:%d\n", hostPort)
		}
		fmt.Printf("   Domain:  http://%s\n", domain)

		// LAN URL
		lanMgr := proxy.NewLanPortManager()
		lanPort := lanMgr.Get(cfg.Name)
		if lanPort > 0 {
			fmt.Printf("   LAN:     http://%s:%d\n", proxy.GetHostIP(), lanPort)
		}

		// Services with URLs
		var svcs []string
		for s, sc := range cfg.Services {
			if sc.Enabled {
				svcs = append(svcs, s)
			}
		}

		if len(svcs) > 0 {
			fmt.Println("\n Services:")
			for _, svc := range svcs {
				fmt.Printf("   - %s", svc)
				switch svc {
				case "mariadb", "mysql":
					fmt.Printf(" (localhost:3306)")
				case "postgres":
					fmt.Printf(" (localhost:5432)")
				case "redis":
					fmt.Printf(" (localhost:6379)")
				case "memcached":
					fmt.Printf(" (localhost:11211)")
				case "mailpit":
					mailpitPort := client.GetHostPort(cname, "8025")
					if mailpitPort > 0 {
						fmt.Printf(" → http://localhost:%d", mailpitPort)
					}
					fmt.Printf(" | http://%s-mailpit.%s", cfg.Name, globalCfg.DomainSuffix)
				case "mailhog":
					mailpitPort := client.GetHostPort(cname, "8025")
					if mailpitPort > 0 {
						fmt.Printf(" → http://localhost:%d", mailpitPort)
					}
					fmt.Printf(" | http://%s-mailhog.%s", cfg.Name, globalCfg.DomainSuffix)
				case "adminer":
					adminerPort := client.GetHostPort(cname, "8080")
					if adminerPort > 0 {
						fmt.Printf(" → http://localhost:%d", adminerPort)
					}
					fmt.Printf(" | http://%s-adminer.%s", cfg.Name, globalCfg.DomainSuffix)
				case "phpmyadmin":
					fmt.Printf(" → http://%s-phpmyadmin.%s", cfg.Name, globalCfg.DomainSuffix)
				case "elasticsearch":
					fmt.Printf(" (localhost:9200)")
				case "rabbitmq":
					fmt.Printf(" (localhost:5672, management:15672)")
				}
				fmt.Println()
			}
		}

		// Database credentials
		fmt.Println("\n Database:")
		dbHost := cfg.Env["DB_HOST"]
		if dbHost == "" {
			dbHost = "localhost"
		}
		dbUser := cfg.Env["DB_USERNAME"]
		if dbUser == "" {
			dbUser = "app"
		}
		dbPass := cfg.Env["DB_PASSWORD"]
		if dbPass == "" {
			dbPass = "app"
		}
		dbName := cfg.Env["DB_DATABASE"]
		if dbName == "" {
			dbName = "app"
		}
		fmt.Printf("   Host:     %s\n", dbHost)
		fmt.Printf("   User:     %s\n", dbUser)
		fmt.Printf("   Password: %s\n", dbPass)
		fmt.Printf("   Database: %s\n", dbName)

		// Tooling
		if len(cfg.Tooling) > 0 {
			fmt.Println("\n Tooling:")
			for name, t := range cfg.Tooling {
				fmt.Printf("   tavpbox %s → %s\n", name, t.Cmd)
			}
		}

		fmt.Println("\n SSH: tavpbox ssh")
		fmt.Println(" Logs: tavpbox logs")

		return nil
	},
}

var logsCmd = &cobra.Command{
	Use:   "logs",
	Short: "Show container logs",
	RunE: func(cmd *cobra.Command, args []string) error {
		_, cfg, err := config.FindProject()
		if err != nil {
			return err
		}
		client := podman.New()
		cname := client.ContainerName(cfg.Name)

		output, err := client.Logs(cname, 50)
		if err != nil {
			return err
		}
		fmt.Print(output)
		return nil
	},
}
