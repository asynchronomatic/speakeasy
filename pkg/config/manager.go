package config

import (
	"os"
	"path"
	"strings"
	"sync"

	"github.com/goccy/go-yaml"
	"github.com/jinzhu/copier"
	"github.com/negrel/assert"
)

type ManagerProvider interface {
	UpdateConfig(updateFunc func(config *Config) error) error
	ReadConfig(readOnly func(config *Config) error) error
}

var DefaultConfigPath = "config.yaml"
var DefaultNodePath = "node.key"
var DefaultRelayPath = "relay.key"
var DefaultAdminDBPath = "admin.jkv"

func SetConfigPath(p string) {
	p = path.Clean(p)
	DefaultConfigPath = path.Join(p, "config.yaml")
	DefaultNodePath = path.Join(p, "node.key")
	DefaultRelayPath = path.Join(p, "relay.key")
	DefaultAdminDBPath = path.Join(p, "admin.jkv")

}

func init() {
	if p := strings.TrimSpace(os.Getenv("SPEAKEASY_CONFIG_PATH")); p != "" {
		SetConfigPath(p)
	} else {
		SetConfigPath(".")
	}
}

// Manager manages the proxy configuration file
type Manager struct {
	configPath string
	config     *Config
	lock       sync.RWMutex
}

// deferredLoadConfig load config if not loaded yet, manager lock must be held
func (m *Manager) deferredLoadConfig() error {
	if m.config == nil {
		config := &Config{}
		data, err := os.ReadFile(m.configPath)
		if err != nil {
			return err
		}
		if err := yaml.Unmarshal(data, config); err != nil {
			return err
		}

		ApplyConfigDefaults(config)
		m.config = config
	}
	return nil
}

// flushConfig Manager lock should be held
func (m *Manager) flushConfig(newConfig *Config) error {
	data, err := yaml.Marshal(newConfig)
	if err != nil {
		return err
	}
	if err = os.WriteFile(m.configPath, data, 0o600); err != nil {
		return err
	}
	m.config = newConfig
	return nil
}

func (m *Manager) UpdateConfig(updateFunc func(config *Config) error) error {
	m.lock.Lock()
	defer m.lock.Unlock()
	if err := m.deferredLoadConfig(); err != nil {
		return err
	}

	newConfig := &Config{}
	err := copier.Copy(newConfig, m.config)
	assert.NoError(err, "could not copy config")

	err = updateFunc(newConfig)
	if err != nil {
		return err
	}

	return m.flushConfig(newConfig)
}

func (m *Manager) ReadConfig(readOnly func(config *Config) error) error {
	m.lock.RLock()
	defer m.lock.RUnlock()
	if err := m.deferredLoadConfig(); err != nil {
		return err
	}

	newConfig := &Config{}
	err := copier.Copy(newConfig, m.config)
	assert.NoError(err, "could not copy config")

	// FIXME: make a copy of our config and throw away any changes
	// also assert ig it was changed
	return readOnly(newConfig)
}

// EnsureLoaded ensures that a config has been loaded
func (m *Manager) EnsureLoaded() error {
	m.lock.Lock()
	defer m.lock.Unlock()
	return m.deferredLoadConfig()
}

// InitializeFromDefaults a config from defaults
func (m *Manager) InitializeFromDefaults() error {
	cfg := &Config{}
	cfg.Proxy.Listen = DefaultProxyListen
	cfg.Proxy.AllowPrivateBackends = true
	cfg.Admin.AdminPort = DefaultAdminPort
	cfg.Admin.RelayPort = DefaultRelayPort
	cfg.Admin.PublicAddress = "auto"
	cfg.Mesh.PublicAddress = "auto"
	cfg.Mesh.Port = 0
	cfg.Mesh.ForcePrivate = false
	cfg.Mesh.MDNSEnabled = true
	cfg.Mesh.Name, _ = os.Hostname()
	cfg.Providers = []Provider{
		{
			ID:        "localhost",
			Type:      "ollama",
			BaseURL:   "http://127.0.0.1:11434",
			Discovery: "pinned",
		},
	}
	m.lock.Lock()
	defer m.lock.Unlock()
	m.config = cfg
	return m.flushConfig(m.config)
}

// Flush the config file to disk
func (m *Manager) Flush() error {
	m.lock.Lock()
	defer m.lock.Unlock()
	return m.flushConfig(m.config)
}

func NewManager(path string) *Manager {
	return &Manager{configPath: path, config: nil}
}
