package core

import (
	"os"
	"strings"

	"github.com/goccy/go-yaml"

	"github.com/asynchronomatic/speakeasy/pkg/log"
)

var DefaultRelayPort = 4001
var DefaultAdminPort = 4002
var DefaultProxyListen = "127.0.0.1:4080"

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

type Config struct {
	Proxy struct {
		Listen               string `yaml:"listen"`
		Theme                string `yaml:"theme,omitempty"`
		Password             string `yaml:"password"`
		AllowPrivateBackends bool   `yaml:"allow_private_backends"`
	} `yaml:"proxy"`
	Admin     AdminConfig `yaml:"admin"`
	Mesh      MeshConfig  `yaml:"mesh"`
	Providers []Provider  `yaml:"providers"`
	Debug     bool        `yaml:"debug"`
}

func applyConfigDefaults(config *Config) {
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

func LoadConfigFile() (*Config, error) {
	config := &Config{}
	data, err := os.ReadFile("config.yaml")
	if err != nil {
		return nil, err
	}
	if err := yaml.Unmarshal(data, config); err != nil {
		return nil, err
	}
	return config, nil
}

func SaveConfig(config *Config) error {
	data, err := yaml.Marshal(config)
	if err != nil {
		return err
	}
	return os.WriteFile("config.yaml", data, 0o600)
}

func LoadConfig() (*Config, error) {
	config, err := LoadConfigFile()
	if err != nil {
		return nil, err
	}
	applyConfigDefaults(config)
	return config, nil
}

func MustLoadConfig() *Config {
	config, err := LoadConfig()
	if err != nil {
		log.Fatalf("Could not load config.yaml  (Err:%v)\n", err)
	}

	if config.Mesh.Address == "" {
		log.Fatalf("Mesh.Address must be set in config.yaml")
	}

	return config
}
