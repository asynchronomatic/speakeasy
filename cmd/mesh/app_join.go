package main

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"charm.land/huh/v2"
	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/asynchronomatic/speakeasy/api"
	"github.com/asynchronomatic/speakeasy/pkg/config"
	"github.com/asynchronomatic/speakeasy/pkg/mesh"
	"github.com/asynchronomatic/speakeasy/pkg/security"
)

var ErrAborted = errors.New("aborted")

func runJoin(inviteURL string) error {
	inviteURL = strings.TrimSpace(inviteURL)
	if inviteURL == "" {
		return fmt.Errorf("invite URL is required")
	}
	cont, err := existingJoinConfig()
	if err != nil {
		return err
	}
	if !cont {
		return nil
	}

	if err := joinWithInvite(inviteURL); err != nil {
		return err
	}
	return proxyStart()
}

func existingJoinConfig() (bool, error) {
	cm := config.NewManager(config.DefaultConfigPath)
	if err := cm.EnsureLoaded(); err != nil {
		return true, nil
	}

	err := cm.ReadConfig(func(cfg *config.Config) error {
		cont := false
		err := huh.NewForm(
			huh.NewGroup(
				huh.NewConfirm().
					Title("config.yaml already exists in this directory.").
					Description(existingJoinWarning(config.DefaultConfigPath, cfg)).
					Affirmative("Continue").
					Negative("Abort").
					Value(&cont),
			),
		).WithAccessible(os.Getenv("ACCESSIBLE") != "").Run()

		if aborted(err) {
			fmt.Println("Aborted.")
			return ErrAborted
		}
		if err != nil {
			return err
		}

		if !cont {
			return ErrAborted
		}
		return nil
	})

	if err == nil {
		if err == ErrAborted {
			return false, nil
		}
	}

	return true, err
}

func existingJoinWarning(abs string, cfg *config.Config) string {
	cwd, err := os.Getwd()
	if err != nil {
		cwd = "."
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Found %s\n", abs)
	fmt.Fprintf(&b, "Working directory: %s\n", cwd)
	if cfg != nil && (cfg.Mesh.MeshId != "" || cfg.Mesh.Address != "") {
		fmt.Fprintf(&b, "Current mesh: %s @ %s\n", cfg.Mesh.MeshId, cfg.Mesh.Address)
	}
	b.WriteString("If this is the wrong directory, abort now — redeeming the invite cannot be undone and will replace mesh address, mesh id, and secret in this config.")
	return b.String()
}

func joinWithInvite(link string) error {
	key, err := mesh.LoadOrCreateKey(config.DefaultNodePath)
	if err != nil {
		return fmt.Errorf("node key: %w", err)
	}
	id, err := peer.IDFromPrivateKey(key)
	if err != nil {
		return fmt.Errorf("peer id: %w", err)
	}

	cm := config.NewManager(config.DefaultConfigPath)
	if err = cm.EnsureLoaded(); err != nil {
		err = cm.InitializeFromDefaults()
		if err != nil {
			return fmt.Errorf("initialize config: %w", err)
		}
	}

	err = cm.UpdateConfig(func(cfg *config.Config) error {
		resp, err := api.RedeemInvite(link, api.Node{ID: id.String(), Name: cfg.Mesh.Name})
		if err != nil {
			return fmt.Errorf("redeem invite: %w", err)
		}

		cfg.Mesh.Address = strings.TrimSpace(resp.MeshServer)
		cfg.Mesh.Secret = resp.MeshSecret
		cfg.Mesh.MeshId = resp.MeshId

		err = ensureProxyPassword(cfg)
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return err
	}

	_ = cm.ReadConfig(func(cfg *config.Config) error {
		fmt.Println()
		fmt.Println("Joined mesh.")
		fmt.Printf("  config:   %s\n", config.DefaultConfigPath)
		fmt.Printf("  peer id:  %s\n", id)
		fmt.Printf("  mesh id:  %s\n", cfg.Mesh.MeshId)
		fmt.Printf("  address:  %s\n", cfg.Mesh.Address)
		fmt.Println()
		fmt.Println("Next:")
		fmt.Println("  mesh proxy    # start the local proxy on this mesh")
		return nil
	})

	return err
}

// askPassword is the interactive prompt used when joining without
// proxy.password. Tests replace it.
var askPassword = promptPassword

func ensureProxyPassword(cfg *config.Config) error {
	if cfg == nil {
		return fmt.Errorf("config is required")
	}
	if strings.TrimSpace(cfg.Proxy.Password) != "" {
		return nil
	}
	pw, err := askPassword("Protects the local proxy UI and on this node.")
	if err != nil {
		return err
	}

	encoded, err := security.PasswordHashAndEncode(pw)
	if err != nil {
		return err
	}
	cfg.Proxy.Password = encoded
	return nil
}

func promptPassword(description string) (string, error) {
	var password, confirm string
	err := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Proxy password").
				Description(description).
				EchoMode(huh.EchoModePassword).
				Value(&password).
				Validate(func(s string) error {
					if strings.TrimSpace(s) == "" {
						return fmt.Errorf("password is required")
					}
					return nil
				}),
			huh.NewInput().
				Title("Confirm password").
				EchoMode(huh.EchoModePassword).
				Value(&confirm).
				Validate(func(s string) error {
					if s != password {
						return fmt.Errorf("passwords do not match")
					}
					return nil
				}),
		).Title("Set proxy password").Description("proxy.password is not set in config.yaml."),
	).WithAccessible(os.Getenv("ACCESSIBLE") != "").Run()
	if aborted(err) {
		return "", fmt.Errorf("aborted")
	}
	if err != nil {
		return "", err
	}
	pw := strings.TrimSpace(password)
	if pw == "" {
		return "", fmt.Errorf("password is required")
	}
	return pw, nil
}
