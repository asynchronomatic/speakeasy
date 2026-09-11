package testable

import (
	"sync"

	"github.com/goccy/go-yaml"

	"github.com/asynchronomatic/speakeasy/pkg/config"
)

type ConfigManager struct {
	config *config.Config
	lock   sync.RWMutex
}

func (m *ConfigManager) SetDefaultConfig(content string) error {
	cfg := config.Config{}
	if err := yaml.Unmarshal([]byte(content), &cfg); err != nil {
		return err
	}
	defer m.lock.Unlock()
	m.lock.Lock()
	config.ApplyConfigDefaults(&cfg)
	m.config = &cfg
	return nil
}

func (m *ConfigManager) UpdateConfig(updateFunc func(config *config.Config) error) error {
	m.lock.Lock()
	defer m.lock.Unlock()
	err := updateFunc(m.config)
	if err != nil {
		return err
	}
	return nil
}

func (m *ConfigManager) ReadConfig(readOnly func(config *config.Config) error) error {
	m.lock.RLock()
	defer m.lock.RUnlock()
	return readOnly(m.config)
}

func (m *ConfigManager) Config() *config.Config {
	return m.config
}

func MustConfigManager(content string) *ConfigManager {
	c := ConfigManager{config: &config.Config{}}

	err := c.SetDefaultConfig(content)
	if err != nil {
		panic(err)
	}
	return &c
}
