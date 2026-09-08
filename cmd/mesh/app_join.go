package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"charm.land/huh/v2"
	"github.com/goccy/go-yaml"
	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/asynchronomatic/speakeasy/api"
	"github.com/asynchronomatic/speakeasy/pkg/core"
	"github.com/asynchronomatic/speakeasy/pkg/log"
	"github.com/asynchronomatic/speakeasy/pkg/mesh"
)

func runJoin(args []string) error {
	if len(args) != 1 || strings.TrimSpace(args[0]) == "" {
		return fmt.Errorf("usage: %s join <invite-url>", os.Args[0])
	}
	existing, cont, err := existingJoinConfig()
	if err != nil {
		return err
	}
	if !cont {
		return nil
	}
	if err := joinWithInvite(strings.TrimSpace(args[0]), existing); err != nil {
		return err
	}
	config := core.MustLoadConfig()
	return runProxy(config)
}

func existingJoinConfig() (*core.Config, bool, error) {
	if !fileExists(defaultConfigPath) {
		return nil, true, nil
	}

	abs, err := filepath.Abs(defaultConfigPath)
	if err != nil {
		abs = defaultConfigPath
	}
	cfg, err := core.LoadConfig()
	if err != nil {
		return nil, false, fmt.Errorf("load %s: %w", defaultConfigPath, err)
	}

	cont := false
	err = huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title("config.yaml already exists in this directory.").
				Description(existingJoinWarning(abs, cfg)).
				Affirmative("Continue").
				Negative("Abort").
				Value(&cont),
		),
	).WithAccessible(os.Getenv("ACCESSIBLE") != "").Run()
	if aborted(err) {
		fmt.Println("Aborted.")
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if !cont {
		fmt.Println("Aborted.")
		return nil, false, nil
	}
	return cfg, true, nil
}

func existingJoinWarning(abs string, cfg *core.Config) string {
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

func joinWithInvite(link string, existing *core.Config) error {
	key, err := mesh.LoadOrCreateKey(defaultNodeKeyPath)
	if err != nil {
		return fmt.Errorf("node key: %w", err)
	}
	id, err := peer.IDFromPrivateKey(key)
	if err != nil {
		return fmt.Errorf("peer id: %w", err)
	}

	name, _ := os.Hostname()
	if existing != nil && strings.TrimSpace(existing.Mesh.Name) != "" {
		name = existing.Mesh.Name
	}
	resp, err := api.RedeemInvite(link, api.Node{ID: id.String(), Name: name})
	if err != nil {
		return fmt.Errorf("redeem invite: %w", err)
	}

	cfg := configFromInvite(resp, existing)
	if err := ensureProxyPassword(cfg); err != nil {
		return err
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("encode config: %w", err)
	}
	if err := os.WriteFile(defaultConfigPath, data, 0o600); err != nil {
		return fmt.Errorf("write %s: %w", defaultConfigPath, err)
	}

	log.Infof("wrote %s\n", defaultConfigPath)
	fmt.Println()
	fmt.Println("Joined mesh.")
	fmt.Printf("  config:   %s\n", defaultConfigPath)
	fmt.Printf("  peer id:  %s\n", id)
	fmt.Printf("  mesh id:  %s\n", cfg.Mesh.MeshId)
	fmt.Printf("  address:  %s\n", cfg.Mesh.Address)
	fmt.Println()
	fmt.Println("Next:")
	fmt.Println("  mesh proxy    # start the local proxy on this mesh")
	return nil
}

// askProxyPassword is the interactive prompt used when joining without
// proxy.password. Tests replace it.
var askProxyPassword = promptProxyPassword

func ensureProxyPassword(cfg *core.Config) error {
	if cfg == nil {
		return fmt.Errorf("config is required")
	}
	if strings.TrimSpace(cfg.Proxy.Password) != "" {
		return nil
	}
	pw, err := askProxyPassword()
	if err != nil {
		return err
	}
	pw = strings.TrimSpace(pw)
	if pw == "" {
		return fmt.Errorf("proxy.password is required")
	}
	cfg.Proxy.Password = pw
	return nil
}

func promptProxyPassword() (string, error) {
	var password, confirm string
	err := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Proxy password").
				Description("Protects the local proxy UI and API on this node.").
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
	return strings.TrimSpace(password), nil
}

func configFromInvite(resp *api.RedeemInviteResponse, existing *core.Config) *core.Config {
	var cfg *core.Config
	if existing != nil {
		cp := *existing
		cfg = &cp
	} else {
		cfg = defaultJoinConfig()
	}
	cfg.Mesh.Address = strings.TrimSpace(resp.MeshServer)
	cfg.Mesh.Secret = resp.MeshSecret
	cfg.Mesh.MeshId = resp.MeshId
	return cfg
}

func defaultJoinConfig() *core.Config {
	cfg := &core.Config{}
	cfg.Proxy.Listen = core.DefaultProxyListen
	cfg.Admin.AdminPort = core.DefaultAdminPort
	cfg.Admin.RelayPort = core.DefaultRelayPort
	cfg.Admin.PublicAddress = "auto"
	cfg.Mesh.PublicAddress = "auto"
	cfg.Mesh.Port = 0
	cfg.Mesh.ForcePrivate = false
	cfg.Mesh.MDNSEnabled = true
	cfg.Mesh.Name, _ = os.Hostname()
	cfg.Providers = []core.Provider{{
		ID:        "localhost",
		Type:      "ollama",
		BaseURL:   "http://127.0.0.1:11434",
		Discovery: "pinned",
	}}
	return cfg
}
