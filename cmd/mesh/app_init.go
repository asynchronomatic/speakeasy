package main

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"charm.land/huh/v2"

	"github.com/asynchronomatic/speakeasy/pkg/config"
	"github.com/asynchronomatic/speakeasy/pkg/log"
	"github.com/asynchronomatic/speakeasy/pkg/mesh"
)

// Install defaults. Change these to adjust what `mesh init` offers (and writes
// if the user keeps the pre-filled value).
var (
	//defaultNodeKeyPath  = "node.key"
	//defaultRelayKeyPath = "relay.key"
	//defaultConfigPath   = config.DefaultConfigPath

	defaultProxyListen    = config.DefaultProxyListen
	defaultOllamaVersion  = "0.33.0"
	defaultMeshName       = ""
	defaultAdminAddress   = "http://127.0.0.1:4002"
	defaultAdminPort      = config.DefaultAdminPort
	defaultRelayPort      = config.DefaultRelayPort
	defaultPublicAddress  = "auto"
	defaultAppPort        = 0
	defaultForcePrivate   = false
	defaultMDNSEnabled    = true
	defaultProviderID     = "localhost"
	defaultProviderType   = "ollama"
	defaultProviderURL    = "http://localhost:11434"
	defaultModelDiscovery = "pinned"

	// Empty means generate a random 32-byte hex secret at init time.
	defaultAdminSecret = ""
)

type installSettings struct {
	NodeKeyPath  string
	RelayKeyPath string
	ConfigPath   string

	ProxyListen   string
	OllamaVersion string

	MeshName      string
	AdminAddress  string
	AdminSecret   string
	AdminPort     int
	RelayPort     int
	PublicAddress string
	AppPort       int
	ForcePrivate  bool
	MDNSEnabled   bool

	ProviderID     string
	ProviderType   string
	ProviderURL    string
	ModelDiscovery string
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func generateAdminSecret() (string, error) {
	if defaultAdminSecret != "" {
		return defaultAdminSecret, nil
	}
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func aborted(err error) bool {
	return err != nil && errors.Is(err, huh.ErrUserAborted)
}

func validatePort(s string) error {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil || n < 0 || n > 65535 {
		return fmt.Errorf("enter a port between 0 and 65535")
	}
	return nil
}

func collectInstallSettings() (installSettings, error) {
	s := installSettings{
		NodeKeyPath:    config.DefaultNodePath,
		RelayKeyPath:   config.DefaultRelayPath,
		ConfigPath:     config.DefaultConfigPath,
		ProxyListen:    defaultProxyListen,
		OllamaVersion:  defaultOllamaVersion,
		MeshName:       defaultMeshName,
		AdminAddress:   defaultAdminAddress,
		AdminSecret:    defaultAdminSecret,
		PublicAddress:  defaultPublicAddress,
		ForcePrivate:   defaultForcePrivate,
		MDNSEnabled:    defaultMDNSEnabled,
		ProviderID:     defaultProviderID,
		ProviderType:   defaultProviderType,
		ProviderURL:    defaultProviderURL,
		ModelDiscovery: defaultModelDiscovery,
	}

	adminPort := strconv.Itoa(defaultAdminPort)
	relayPort := strconv.Itoa(defaultRelayPort)
	appPort := strconv.Itoa(defaultAppPort)
	write := true

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Proxy listen address").
				Description("Local HTTP address for the Ollama/OpenAI proxy and the /ui dashboard. Example: :8080").
				Placeholder(defaultProxyListen).
				Value(&s.ProxyListen),
			huh.NewInput().
				Title("Reported Ollama version").
				Description("Version string returned to Ollama-compatible clients.").
				Placeholder(defaultOllamaVersion).
				Value(&s.OllamaVersion),
		).Title("Proxy").Description("How this node presents the local HTTP API."),

		huh.NewGroup(
			huh.NewInput().
				Title("Node name").
				Description("Human-readable name on the mesh. Leave empty to use the hostname at startup.").
				Placeholder("(hostname)").
				Value(&s.MeshName),
			huh.NewInput().
				Title("Admin address").
				Description("HTTP(S) URL of the mesh admin this proxy registers with. The admin node must be reachable from the internet (public host or NAT port-forward of the admin port). For hybrid/standalone on this machine use http://127.0.0.1:<admin_port>.").
				Placeholder(defaultAdminAddress).
				Value(&s.AdminAddress),
			huh.NewInput().
				Title("Admin secret").
				Description("Shared password for the admin API. Leave empty to generate a random secret.").
				Placeholder("generate random").
				Value(&s.AdminSecret),
			huh.NewInput().
				Title("Admin port").
				Description("TCP port the admin controller binds for mesh admin / proxy+admin. Forward this if behind NAT.").
				Placeholder(strconv.Itoa(defaultAdminPort)).
				Value(&adminPort).
				Validate(validatePort),
		).Title("Admin").Description("Membership controller this node talks to. It must be public or port-forwarded."),

		huh.NewGroup(
			huh.NewInput().
				Title("Relay port").
				Description("TCP+UDP port for the libp2p circuit relay (NAT traversal). Forward both protocols on a public relay.").
				Placeholder(strconv.Itoa(defaultRelayPort)).
				Value(&relayPort).
				Validate(validatePort),
			huh.NewInput().
				Title("Public address").
				Description("IP the relay advertises. Use auto to discover it, or a concrete IP when ports are forwarded through NAT.").
				Placeholder(defaultPublicAddress).
				Value(&s.PublicAddress),
			huh.NewInput().
				Title("Mesh app port").
				Description("libp2p listen port. 0 lets a proxy pick an ephemeral port; relay/hybrid nodes typically use the relay port.").
				Placeholder(strconv.Itoa(defaultAppPort)).
				Value(&appPort).
				Validate(validatePort),
			huh.NewConfirm().
				Title("Force private reachability?").
				Description("If yes, this proxy always behaves as if it is behind NAT and prefers hole punching / relay paths.").
				Affirmative("Yes").
				Negative("No").
				Value(&s.ForcePrivate),
			huh.NewConfirm().
				Title("Enable mDNS?").
				Description("Discover other Speakeasy proxies on the local LAN, in addition to the admin membership list.").
				Affirmative("Yes").
				Negative("No").
				Value(&s.MDNSEnabled),
		).Title("Network").Description("How this node joins the libp2p mesh."),

		huh.NewGroup(
			huh.NewInput().
				Title("Local provider id").
				Description("Label for the local Ollama backend in config.yaml.").
				Placeholder(defaultProviderID).
				Value(&s.ProviderID),
			huh.NewSelect[string]().
				Title("Provider type").
				Description("Backend type. Only ollama is implemented today.").
				Options(huh.NewOptions("ollama")...).
				Value(&s.ProviderType),
			huh.NewInput().
				Title("Ollama base URL").
				Description("Where this node’s Ollama HTTP API listens.").
				Placeholder(defaultProviderURL).
				Value(&s.ProviderURL),
			huh.NewSelect[string]().
				Title("Model discovery").
				Description("Which local models to export onto the mesh.").
				Options(
					huh.NewOption("all — every model Ollama knows", "all"),
					huh.NewOption("pinned — only currently loaded models", "pinned"),
					huh.NewOption("whitelist — only names listed under providers[].models", "whitelist"),
				).
				Value(&s.ModelDiscovery),
		).Title("Ollama").Description("Which local models this node shares."),

		huh.NewGroup(
			huh.NewConfirm().
				Title("Write these settings?").
				Description("Creates config.yaml. Existing node.key / relay.key files are kept; missing keys are created.").
				Affirmative("Write").
				Negative("Abort").
				Value(&write),
		).Title("Confirm"),
	).WithAccessible(os.Getenv("ACCESSIBLE") != "")

	if err := form.Run(); err != nil {
		return s, err
	}
	if !write {
		return s, fmt.Errorf("aborted")
	}

	s.AdminPort, _ = strconv.Atoi(strings.TrimSpace(adminPort))
	s.RelayPort, _ = strconv.Atoi(strings.TrimSpace(relayPort))
	s.AppPort, _ = strconv.Atoi(strings.TrimSpace(appPort))

	if strings.TrimSpace(s.AdminSecret) == "" {
		sec, err := generateAdminSecret()
		if err != nil {
			return s, err
		}
		s.AdminSecret = sec
		fmt.Printf("Generated admin_secret: %s\n", s.AdminSecret)
	}

	return s, nil
}

func defaultConfigYAML(s installSettings) string {
	forcePrivate := "false"
	if s.ForcePrivate {
		forcePrivate = "true"
	}
	mdns := "false"
	if s.MDNSEnabled {
		mdns = "true"
	}
	return fmt.Sprintf(`proxy:
  # The listen address for the Ollama proxy endpoint
  listen: %q
  # The version of ollama to report as
  version: %q
  # Allow provider base_url to target loopback / private / link-local addresses.
  # Required for a local Ollama at 127.0.0.1. Leave false on a public listen
  # unless you intentionally proxy to LAN backends.
  allow_private_backends: true

mesh:
  name: %q
  # Admin (Mesh Authentication Controller) API Path and secret
  #  This points at a server that controls access to the mesh for member nodes
  #
  #  admin_address - HTTP(S) endpoint proxies use to register.
  #                  The admin node must be hosted publicly or exposed to the
  #                  internet via port forwarding (TCP on admin_port).
  #  admin_secret  - is the admin controller password
  #  admin_port    - is the port the controller should bind to (if behind nat this is the port to forward to the server)
  admin_address: %q
  admin_secret: %q
  admin_port: %d
  relay_port: %d
  # The public address the relay server should announce itself as
  #   When the server is directly on the internet this can be "" and the public ip will be selected
  #   IF the relay/controller is behind NAT with ports forwarded to it, this should be the address of the
  #   router
  #
  #    "auto" - use an automatic scheme to determine your address
  #
  public_address: %q

  # The app port to bind to for our MESH network
  #   for mesh proxy this can be set to zero
  #   for mesh relay this must be set to a valid port number
  app_port: %d
  # Force "mesh proxy" to think it is behind NAT (for hole punching)
  force_private: %s
  # Enable/Discover of other LOCAL mesh proxy instances using MDNS (On Local Lan)
  mdns_enabled: %s

providers:
  - id: %s
    # only ollama is supported currently
    type: %s
    # Base url of this server
    base_url: %q
    # Controls how models are treated
    # all       - all models that are known to
    # pinned    - only currently loaded models are exported (prevents model thrashing)
    # whitelist - only models in the whitelist (models:) are exported
    model_discovery: %s
    models:
`, s.ProxyListen, s.OllamaVersion, s.MeshName, s.AdminAddress, s.AdminSecret, s.AdminPort, s.RelayPort, s.PublicAddress, s.AppPort, forcePrivate, mdns, s.ProviderID, s.ProviderType, s.ProviderURL, s.ModelDiscovery)
}

func initializeNewInstall() error {
	existing := make([]string, 0, 3)
	for _, path := range []string{config.DefaultNodePath, config.DefaultRelayPath, config.DefaultConfigPath} {
		if fileExists(path) {
			existing = append(existing, path)
		}
	}

	if len(existing) > 0 {
		cont := false
		desc := "Found: " + strings.Join(existing, ", ") + ".\nContinuing will write a new config.yaml. Existing node.key / relay.key files will be kept."
		err := huh.NewForm(
			huh.NewGroup(
				huh.NewConfirm().
					Title("This app is already initialized.").
					Description(desc).
					Affirmative("Continue").
					Negative("Abort").
					Value(&cont),
			),
		).WithAccessible(os.Getenv("ACCESSIBLE") != "").Run()
		if aborted(err) {
			fmt.Println("Aborted.")
			return nil
		}
		if err != nil {
			return err
		}
		if !cont {
			fmt.Println("Aborted.")
			return nil
		}
	}

	settings, err := collectInstallSettings()
	if aborted(err) {
		fmt.Println("Aborted.")
		return nil
	}
	if err != nil {
		if err.Error() == "aborted" {
			fmt.Println("Aborted.")
			return nil
		}
		return err
	}

	if err := os.WriteFile(settings.ConfigPath, []byte(defaultConfigYAML(settings)), 0o600); err != nil {
		return fmt.Errorf("write %s: %w", settings.ConfigPath, err)
	}
	log.Infof("wrote %s\n", settings.ConfigPath)

	if _, err := mesh.LoadOrCreateKey(settings.NodeKeyPath); err != nil {
		return fmt.Errorf("create %s: %w", settings.NodeKeyPath, err)
	}
	log.Infof("node identity: %s\n", settings.NodeKeyPath)

	if _, err := mesh.LoadOrCreateKey(settings.RelayKeyPath); err != nil {
		return fmt.Errorf("create %s: %w", settings.RelayKeyPath, err)
	}
	log.Infof("relay identity: %s\n", settings.RelayKeyPath)

	fmt.Println()
	fmt.Println("Install ready.")
	fmt.Printf("  config:     %s\n", settings.ConfigPath)
	fmt.Printf("  node key:   %s\n", settings.NodeKeyPath)
	fmt.Printf("  relay key:  %s\n", settings.RelayKeyPath)
	fmt.Println()
	fmt.Println("Next:")
	fmt.Println("  mesh join           # point this node at an existing admin")
	fmt.Println("  mesh proxy          # start the local proxy")
	fmt.Println("  mesh admin          # run admin + relay")
	fmt.Println("  mesh proxy+admin    # standalone")
	return nil
}
