package config

import (
	"os"
	"path"
	"path/filepath"

	"github.com/pkg/errors"
	"gopkg.in/yaml.v3"
)

type Config struct {
	ApiUrl              string `yaml:"api_url" json:"api_url"`
	DefaultOrg          string `yaml:"default_org_id" json:"default_org_id"`
	Token               string `yaml:"token" json:"token"`
	DisableVersionCheck *bool  `yaml:"disable_version_check,omitempty" json:"disable_version_check,omitempty"`
}

func SaveFile(config Config) error {
	f, err := yaml.Marshal(config)
	if err != nil {
		return errors.Wrap(err, "failed to marshal config")
	}
	configFilePath, err := FilePath()
	if err != nil {
		return errors.Wrap(err, "failed to get config file path")
	}
	if err := os.MkdirAll(path.Dir(configFilePath), 0750); err != nil {
		return errors.Wrap(err, "failed to create config directory")
	}
	if err := os.WriteFile(configFilePath+".tmp", f, 0600); err != nil {
		return errors.Wrap(err, "failed to write config file")
	}
	if err := os.Rename(configFilePath+".tmp", configFilePath); err != nil {
		return errors.Wrap(err, "failed to rename config file to final location")
	}
	return nil
}

func ReadFile() (Config, error) {
	configFilePath, err := FilePath()
	if err != nil {
		return Config{}, errors.Wrap(err, "failed to get config file path")
	}
	var cfg Config
	f, err := os.ReadFile(filepath.Clean(configFilePath))
	if err != nil {
		if os.IsNotExist(err) {
			// If the file does not exist, return an empty config
			return cfg, nil
		}
		return cfg, errors.Wrap(err, "failed to read config file")
	}
	if err := yaml.Unmarshal(f, &cfg); err != nil {
		return cfg, errors.Wrap(err, "failed to parse config file")
	}
	return cfg, nil
}

func FilePath() (string, error) {
	configDir, err := Dir()
	if err != nil {
		return "", errors.Wrap(err, "failed to get config directory")
	}
	return path.Join(configDir, "config.yaml"), nil
}

func Dir() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(homeDir, ".config", "octl"), nil
}
