package config

import (
	"os"
	"strings"
	"time"
)

var DefaultRelayPort = 4001
var DefaultAdminPort = 4002
var DefaultProxyListen = ":4080"

const DefaultTheme = "deco"

var AllowedThemes = []string{"night", "deco", "cyber", "clean"}

func NormalizeTheme(name string) string {
	n := strings.ToLower(strings.TrimSpace(name))
	for _, t := range AllowedThemes {
		if n == t {
			return t
		}
	}
	return DefaultTheme
}

type ModelConfig struct {
	Model        string   `yaml:"model" json:"model"`
	Private      bool     `yaml:"private" json:"private"`
	Capabilities []string `yaml:"capabilities" json:"capabilities"`
	Tools        []string `yaml:"tools" json:"tools"`
}

type Provider struct {
	ID        string        `yaml:"id" json:"id"`
	Type      string        `yaml:"type" json:"type"`
	BaseURL   string        `yaml:"base_url" json:"base_url"`
	Token     string        `yaml:"token" json:"token"`
	Private   bool          `yaml:"private" json:"private"`
	Discovery string        `yaml:"model_discovery" json:"model_discovery"`
	Models    []ModelConfig `yaml:"models" json:"models"`
}

type EnabledModel struct {
	Name            string
	Strategy        string
	SessionAffinity string
	Targets         []struct {
		Model   string
		BaseURL string `yaml:"base_url"`
		Weight  int
	}
}

type AdminConfig struct {
	Address       string `yaml:"address"`
	Secret        string `yaml:"secret"`
	AdminPort     int    `yaml:"admin_port"`
	RelayPort     int    `yaml:"relay_port"`
	PublicAddress string `yaml:"public_address"`
}

type MeshConfig struct {
	Name          string `yaml:"name"`
	Address       string `yaml:"address"`
	Secret        string `yaml:"secret"`
	MeshId        string `yaml:"mesh_id"`
	PublicAddress string `yaml:"public_address"`
	Port          int    `yaml:"port"`
	ForcePrivate  bool   `yaml:"force_private"`
	MDNSEnabled   bool   `yaml:"mdns_enabled"`
}

type InferenceToken struct {
	Name    string    `yaml:"name"`  // friendly name for the token
	Token   string    `yaml:"token"` // bcrypt hash of the secret
	Created time.Time `yaml:"created"`
}

type Config struct {
	Proxy struct {
		Listen               string `yaml:"listen"`
		Theme                string `yaml:"theme,omitempty"`
		Password             string `yaml:"password"`
		AllowPrivateBackends bool   `yaml:"allow_private_backends"`
		InferenceTokens      struct {
			Insecure bool             `yaml:"insecure"` // allows insecure inference to the proxy
			Tokens   []InferenceToken `yaml:"tokens"`
		} `yaml:"inference_tokens"`
	} `yaml:"proxy"`
	Admin     AdminConfig `yaml:"admin"`
	Mesh      MeshConfig  `yaml:"mesh"`
	Providers []Provider  `yaml:"providers"`
	Debug     bool        `yaml:"debug"`
}

func ApplyConfigDefaults(config *Config) {
	if config.Proxy.Listen == "" {
		config.Proxy.Listen = DefaultProxyListen
	}
	if config.Admin.RelayPort <= 0 {
		config.Admin.RelayPort = DefaultRelayPort
	}
	if config.Mesh.Port <= 0 {
		config.Mesh.Port = 0
	}
	if config.Admin.AdminPort <= 0 {
		config.Admin.AdminPort = DefaultAdminPort
	}
	if config.Mesh.Name == "" {
		config.Mesh.Name, _ = os.Hostname()
	}
	config.Proxy.Theme = NormalizeTheme(config.Proxy.Theme)
}
