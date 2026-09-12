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
	keyExists := fileExists(config.DefaultNodePath)

	cm := config.NewManager(config.DefaultConfigPath)
	if err := cm.EnsureLoaded(); err == nil {
		err = cm.UpdateConfig(func(cfg *config.Config) error {
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
				return fmt.Errorf("aborted")
			}

			if cfg != nil {
				clearMembership(cfg)
				log.Infof("cleared membership settings in %s\n", config.DefaultConfigPath)
			}
			return nil
		})
		if err != nil {
			return err
		}
	}

	if err := removeFile(config.DefaultNodePath); err != nil {
		return err
	}

	fmt.Println()
	fmt.Println("Reset complete.")
	fmt.Printf("  cleared:  admin.address, admin.secret, mesh.address, mesh.mesh_id in %s\n", config.DefaultConfigPath)
	if keyExists {
		fmt.Printf("  deleted:  %s\n", config.DefaultNodePath)
	}
	fmt.Println()
	fmt.Println("Next:")
	fmt.Println("  mesh join <invite-url>    # rejoin a mesh (creates a new node.key)")
	return nil
}

func resetAll() error {
	configExists := fileExists(config.DefaultConfigPath)
	keyExists := fileExists(config.DefaultNodePath)

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

	if err := removeFile(config.DefaultConfigPath); err != nil {
		return err
	}
	if err := removeFile(config.DefaultNodePath); err != nil {
		return err
	}
	if err := removeFile(config.DefaultRelayPath); err != nil {
		return err
	}

	if err := removeDir(config.DefaultAdminDBPath); err != nil {
		return err
	}

	fmt.Println()
	fmt.Println("Reset complete.")
	if configExists {
		fmt.Printf("  deleted:  %s\n", config.DefaultConfigPath)
	}
	if keyExists {
		fmt.Printf("  deleted:  %s\n", config.DefaultNodePath)
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
		fmt.Fprintf(&b, "  %s  (this node's libp2p identity; a new one is created on the next join)\n", config.DefaultNodePath)
	} else {
		fmt.Fprintf(&b, "  %s  (not present)\n", config.DefaultNodePath)
	}
	b.WriteString("\nYou will need to run mesh join <invite-url> to rejoin.")
	return b.String()
}

func allResetDescription(configExists, keyExists bool) string {
	var b strings.Builder
	b.WriteString("This deletes the local install files. relay.key is kept.\n\nWill delete:\n")
	if configExists {
		fmt.Fprintf(&b, "  %s  (proxy, providers, admin, and mesh settings)\n", config.DefaultConfigPath)
	} else {
		fmt.Fprintf(&b, "  %s  (not present)\n", config.DefaultConfigPath)
	}
	if keyExists {
		fmt.Fprintf(&b, "  %s  (this node's libp2p identity)\n", config.DefaultNodePath)
	} else {
		fmt.Fprintf(&b, "  %s  (not present)\n", config.DefaultNodePath)
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

func removeDir(path string) error {
	if err := os.RemoveAll(path); err != nil && !os.IsNotExist(err) {
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
