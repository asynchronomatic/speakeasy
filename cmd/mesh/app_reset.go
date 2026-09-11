package main

import (
	"fmt"
	"os"
	"strings"

	"charm.land/huh/v2"

	"github.com/asynchronomatic/speakeasy/pkg/config"
	"github.com/asynchronomatic/speakeasy/pkg/log"
)

var confirmReset = promptResetConfirm

func runReset(all bool) error {
	if all {
		return resetAll()
	}
	return resetMembership()
}

func resetMembership() error {
	configExists := fileExists(defaultConfigPath)
	keyExists := fileExists(defaultNodeKeyPath)

	var cfg *config.Config
	if configExists {
		var err error
		cfg, err = config.LoadConfigFile()
		if err != nil {
			return fmt.Errorf("load %s: %w", defaultConfigPath, err)
		}
	}

	if !hasMembership(cfg) && !keyExists {
		fmt.Println("Nothing to reset: no mesh membership settings or node.key in this directory.")
		return nil
	}

	ok, err := confirmReset("Detach this node from its mesh?", membershipResetDescription(cfg, keyExists))
	if err != nil {
		return err
	}
	if !ok {
		fmt.Println("Aborted.")
		return nil
	}

	if cfg != nil {
		clearMembership(cfg)
		if err := config.SaveConfig(cfg); err != nil {
			return fmt.Errorf("write %s: %w", defaultConfigPath, err)
		}
		log.Infof("cleared membership settings in %s\n", defaultConfigPath)
	}
	if err := removeFile(defaultNodeKeyPath); err != nil {
		return err
	}

	fmt.Println()
	fmt.Println("Reset complete.")
	if cfg != nil {
		fmt.Printf("  cleared:  admin.address, admin.secret, mesh.address, mesh.mesh_id in %s\n", defaultConfigPath)
	}
	if keyExists {
		fmt.Printf("  deleted:  %s\n", defaultNodeKeyPath)
	}
	fmt.Println()
	fmt.Println("Next:")
	fmt.Println("  mesh join <invite-url>    # rejoin a mesh (creates a new node.key)")
	return nil
}

func resetAll() error {
	configExists := fileExists(defaultConfigPath)
	keyExists := fileExists(defaultNodeKeyPath)
	if !configExists && !keyExists {
		fmt.Println("Nothing to reset: config.yaml and node.key are not in this directory.")
		return nil
	}

	ok, err := confirmReset("Delete this node's config and identity?", allResetDescription(configExists, keyExists))
	if err != nil {
		return err
	}
	if !ok {
		fmt.Println("Aborted.")
		return nil
	}

	if err := removeFile(defaultConfigPath); err != nil {
		return err
	}
	if err := removeFile(defaultNodeKeyPath); err != nil {
		return err
	}

	fmt.Println()
	fmt.Println("Reset complete.")
	if configExists {
		fmt.Printf("  deleted:  %s\n", defaultConfigPath)
	}
	if keyExists {
		fmt.Printf("  deleted:  %s\n", defaultNodeKeyPath)
	}
	fmt.Println()
	fmt.Println("Next:")
	fmt.Println("  mesh init                 # write a new config.yaml")
	fmt.Println("  mesh join <invite-url>    # join a mesh")
	return nil
}

func hasMembership(cfg *config.Config) bool {
	if cfg == nil {
		return false
	}
	return strings.TrimSpace(cfg.Admin.Address) != "" ||
		strings.TrimSpace(cfg.Admin.Secret) != "" ||
		strings.TrimSpace(cfg.Mesh.Address) != "" ||
		strings.TrimSpace(cfg.Mesh.MeshId) != ""
}

func clearMembership(cfg *config.Config) {
	cfg.Admin.Address = ""
	cfg.Admin.Secret = ""
	cfg.Mesh.Address = ""
	cfg.Mesh.MeshId = ""
}

func membershipResetDescription(cfg *config.Config, keyExists bool) string {
	var b strings.Builder
	b.WriteString("This detaches the node from its current mesh. Proxy settings, providers, and relay.key are kept.\n")
	if cfg != nil {
		b.WriteString("\nWill clear in config.yaml:\n")
		fmt.Fprintf(&b, "  admin.address  %s\n", displayValue(cfg.Admin.Address))
		fmt.Fprintf(&b, "  admin.secret   %s\n", displaySecret(cfg.Admin.Secret))
		fmt.Fprintf(&b, "  mesh.address   %s\n", displayValue(cfg.Mesh.Address))
		fmt.Fprintf(&b, "  mesh.mesh_id   %s\n", displayValue(cfg.Mesh.MeshId))
	}
	b.WriteString("\nWill delete:\n")
	if keyExists {
		fmt.Fprintf(&b, "  %s  (this node's libp2p identity; a new one is created on the next join)\n", defaultNodeKeyPath)
	} else {
		fmt.Fprintf(&b, "  %s  (not present)\n", defaultNodeKeyPath)
	}
	b.WriteString("\nYou will need to run mesh join <invite-url> to rejoin.")
	return b.String()
}

func allResetDescription(configExists, keyExists bool) string {
	var b strings.Builder
	b.WriteString("This deletes the local install files. relay.key is kept.\n\nWill delete:\n")
	if configExists {
		fmt.Fprintf(&b, "  %s  (proxy, providers, admin, and mesh settings)\n", defaultConfigPath)
	} else {
		fmt.Fprintf(&b, "  %s  (not present)\n", defaultConfigPath)
	}
	if keyExists {
		fmt.Fprintf(&b, "  %s  (this node's libp2p identity)\n", defaultNodeKeyPath)
	} else {
		fmt.Fprintf(&b, "  %s  (not present)\n", defaultNodeKeyPath)
	}
	b.WriteString("\nYou will need to run mesh init and mesh join to start over.")
	return b.String()
}

func displayValue(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "(empty)"
	}
	return s
}

func displaySecret(s string) string {
	if strings.TrimSpace(s) == "" {
		return "(empty)"
	}
	return "(set)"
}

func removeFile(path string) error {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove %s: %w", path, err)
	}
	return nil
}

func promptResetConfirm(title, description string) (bool, error) {
	ok := false
	err := huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title(title).
				Description(description).
				Affirmative("Reset").
				Negative("Abort").
				Value(&ok),
		),
	).WithAccessible(os.Getenv("ACCESSIBLE") != "").Run()
	if aborted(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return ok, nil
}
