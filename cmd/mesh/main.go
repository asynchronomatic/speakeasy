package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/asynchronomatic/speakeasy/pkg/autoip"
	"github.com/asynchronomatic/speakeasy/pkg/core"
	"github.com/asynchronomatic/speakeasy/pkg/log"
)

func discoverPublicAddress(config *core.Config) string {
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
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage:", os.Args[0], "join <invite-url> | proxy | admin ")
		os.Exit(1)
	}

	cmd := strings.ToLower(os.Args[1])

	var err error
	switch cmd {
	case "join":
		if err := runJoin(os.Args[2:]); err != nil {
			log.Fatalf("Error running mesh: %v\n", err)
		}

	case "proxy":
		config := core.MustLoadConfig()
		err = runProxy(config)

	case "admin":
		config := core.MustLoadConfig()
		err = runAdminAndRelay(config)

	default:
		fmt.Fprintln(os.Stderr, "usage:", os.Args[0], "init | join <invite-url> | proxy | admin | hybrid(proxy+admin)")
		os.Exit(1)
	}

	if err != nil {
		log.Fatalf("Error running mesh: %v\n", err)
	}
}
