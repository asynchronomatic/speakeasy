package config

import (
	"os"
	"strings"
	"sync"

	"github.com/goccy/go-yaml"
)

type ManagerProvider interface {
	UpdateConfig(updateFunc func(config *Config) error) error
	ReadConfig(readOnly func(config *Config) error) error
}

var DefaultConfigPath = "config.yaml"

func init() {
	if p := strings.TrimSpace(os.Getenv("SPEAKEASY_CONFIG")); p != "" {
		DefaultConfigPath = p
	}
}

// Manager manages the proxy configuration file
type Manager struct {
	configPath string
	config     *Config
	lock       sync.RWMutex
}

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

func (m *Manager) UpdateConfig(updateFunc func(config *Config) error) error {
	m.lock.Lock()
	defer m.lock.Unlock()
	if err := m.deferredLoadConfig(); err != nil {
		return err
	}

	err := updateFunc(m.config)
	if err != nil {
		return err
	}

	data, err := yaml.Marshal(m.config)
	if err != nil {
		return err
	}
	return os.WriteFile(m.configPath, data, 0o600)
}

func (m *Manager) ReadConfig(readOnly func(config *Config) error) error {
	m.lock.RLock()
	defer m.lock.RUnlock()
	if err := m.deferredLoadConfig(); err != nil {
		return err
	}

	// FIXME: make a copy of our config and throw away any changes
	// also assert ig it was changed
	return readOnly(m.config)
}

func (m *Manager) LoadConfig() error {
	m.lock.Lock()
	defer m.lock.Unlock()
	return m.deferredLoadConfig()
}

// Flush the config file to disk
func (m *Manager) Flush() error {
	defer m.lock.Unlock()
	m.lock.Lock()
	return SaveConfig(m.config)
}

func NewManager(path string) *Manager {
	return &Manager{configPath: path, config: nil}
}
