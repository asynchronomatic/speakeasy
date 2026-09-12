package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/urfave/cli/v3"

	"github.com/asynchronomatic/speakeasy/pkg/autoip"
	"github.com/asynchronomatic/speakeasy/pkg/config"
	"github.com/asynchronomatic/speakeasy/pkg/log"
)

func discoverPublicAddress(config *config.Config) string {
	publicAddress := config.Mesh.PublicAddress
	switch publicAddress {
	case "", "auto":
		dc, err := autoip.GetPublicAddress()
		if err != nil {
			fmt.Printf("Error: Could not get discover the public address.\n")
			fmt.Printf("       %v\n\n", err)
			fmt.Printf("The Admin/Relay server needs a public ip address to accept connections from the mesh.\n")
			fmt.Printf("You could try setting the expected public address by setting public_address to the ip address of the machine.\n")
			return ""
		}

		if dc.IsPublic() {
			publicAddress = dc.Public
			break
		}

		if dc.IsNAT() {
			publicAddress = dc.Public
			fmt.Printf("Warning: We discovered a public address of %s but this does not match any of the machine addresses\n", dc.Public)
			fmt.Printf("  For mesh proxies to ber able to communicate you must port forward %d(tcp),%d(tcp+udp) top %s\n",
				config.Admin.AdminPort, config.Admin.RelayPort, dc.Outbound)
		}
	}

	log.WithName("main").Highlightf("Public Address: %s\n", publicAddress)
	return publicAddress
}

func main() {
	if err := newCommand().Run(context.Background(), os.Args); err != nil {
		log.Fatalf("Error running mesh: %v\n", err)
	}
}

func newCommand() *cli.Command {
	return &cli.Command{
		Name:                  "mesh",
		Usage:                 "Speakeasy mesh node: init, join, proxy, and admin",
		EnableShellCompletion: true,
		Commands: []*cli.Command{
			{
				Name:  "init",
				Usage: "Interactive setup: write config.yaml, node.key, and relay.key",
				Action: func(context.Context, *cli.Command) error {
					return initializeNewInstall()
				},
			},
			{
				Name:      "join",
				Usage:     "Join a mesh from an invite URL, then start the proxy",
				ArgsUsage: "INVITE_URL",
				Action: func(_ context.Context, cmd *cli.Command) error {
					url, err := requireArg(cmd, "invite URL")
					if err != nil {
						return err
					}
					return runJoin(url)
				},
			},
			{
				Name:  "proxy",
				Usage: "Start the local OpenAI/Ollama proxy on this mesh",
				Action: func(context.Context, *cli.Command) error {
					return runProxy()
				},
			},
			{
				Name:  "admin",
				Usage: "Run the admin HTTP API and libp2p relay",
				Action: func(context.Context, *cli.Command) error {
					return runAdminAndRelay()
				},
			},
			{
				Name:    "hybrid",
				Aliases: []string{"standalone", "proxy+admin"},
				Usage:   "Run admin, relay, and proxy on one machine",
				Action: func(context.Context, *cli.Command) error {
					return runHybrid()
				},
			},
			{
				Name:  "reset",
				Usage: "Detach this node from its mesh (clear membership and delete node.key)",
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:  "all",
						Usage: "Delete config.yaml and node.key entirely",
					},
				},
				Action: func(_ context.Context, cmd *cli.Command) error {
					return runReset(cmd.Bool("all"))
				},
			},
		},
	}
}

func requireArg(cmd *cli.Command, name string) (string, error) {
	v := strings.TrimSpace(cmd.Args().First())
	if v == "" {
		return "", fmt.Errorf("%s is required", name)
	}
	return v, nil
}
