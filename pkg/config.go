package pkg

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	MinimaxAPIKey string `yaml:"minimax_apikey"`
	NotionAPIKey  string `yaml:"notion_apikey"`
}

var config *Config

func LoadConfig() error {
	data, err := os.ReadFile("env4config.yml")
	if err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("failed to parse config: %w", err)
	}

	config = &cfg
	return nil
}

func GetConfig() *Config {
	if config == nil {
		if err := LoadConfig(); err != nil {
			return nil
		}
	}
	return config
}

func GetMinimaxAPIKey() string {
	if apiKey := os.Getenv("MINIMAX_API_KEY"); apiKey != "" {
		return apiKey
	}
	if cfg := GetConfig(); cfg != nil && cfg.MinimaxAPIKey != "" {
		return cfg.MinimaxAPIKey
	}
	return ""
}

func GetNotionAPIKey() string {
	if apiKey := os.Getenv("NOTION_API_KEY"); apiKey != "" {
		return apiKey
	}
	if cfg := GetConfig(); cfg != nil && cfg.NotionAPIKey != "" {
		return cfg.NotionAPIKey
	}
	return ""
}
