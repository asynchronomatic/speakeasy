package config

import (
	"sync"

	"github.com/asynchronomatic/speakeasy/pkg/core"
)

// Manager manages the proxy configuration file
type Manager struct {
	configPath string
	config     *core.Config
	lock       sync.RWMutex
}

func (m *Manager) AtomicUpdate(updateFunc func(config *core.Config) error) error {
	m.lock.Lock()
	defer m.lock.Unlock()
	err := updateFunc(m.config)
	if err != nil {
		return err
	}
	return core.SaveConfig(m.config)
}

// Flush the config file to disk
func (m *Manager) Flush() error {
	defer m.lock.Unlock()
	m.lock.Lock()
	return core.SaveConfig(m.config)
}

func NewManager(path string) (*Manager, error) {
	cfg, err := core.LoadConfigFile()
	if err != nil {
		return nil, err
	}
	return &Manager{configPath: path, config: cfg}, nil
}
