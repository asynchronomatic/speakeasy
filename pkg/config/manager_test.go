package config

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewManager(t *testing.T) {
	dir := t.TempDir()
	SetConfigPath(dir)
	cm := NewManager(DefaultConfigPath)

	err := cm.InitializeFromDefaults()
	assert.NoError(t, err)

	err = cm.ReadConfig(func(config *Config) error {
		config.Mesh.Name = "updated-mesh"
		return nil
	})
	assert.NoError(t, err)

	err = cm.ReadConfig(func(config *Config) error {
		assert.NotEqual(t, "updated-mesh", config.Mesh.Name)
		return nil
	})
	assert.NoError(t, err)

	err = cm.UpdateConfig(func(config *Config) error {
		config.Mesh.Name = "updated-mesh"
		return nil
	})
	assert.NoError(t, err)
	err = cm.ReadConfig(func(config *Config) error {
		assert.Equal(t, "updated-mesh", config.Mesh.Name)
		return nil
	})

	err = cm.UpdateConfig(func(config *Config) error {
		config.Mesh.Name = "new-mesh"
		return fmt.Errorf("error")
	})
	assert.Error(t, err)

	err = cm.ReadConfig(func(config *Config) error {
		assert.Equal(t, "updated-mesh", config.Mesh.Name)
		return nil
	})

}
