package conf

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	MinimaxAPIKey string       `yaml:"minimax_apikey"`
	NotionAPIKey  string       `yaml:"notion_apikey"`
	Chroma        ChromaConfig `yaml:"chroma"`
}

type ChromaConfig struct {
	Host       string `yaml:"host"`
	Port       int    `yaml:"port"`
	Collection string `yaml:"collection"`
}

var config *Config

func getConfigPath() string {
	configPaths := []string{
		"env4config.yml",
		"./conf/env4config.yml",
		"../env4config.yml",
	}

	for _, path := range configPaths {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}

	exePath, err := os.Executable()
	if err == nil {
		exeDir := filepath.Dir(exePath)
		candidatePath := filepath.Join(exeDir, "env4config.yml")
		if _, err := os.Stat(candidatePath); err == nil {
			return candidatePath
		}
	}

	return "env4config.yml"
}

func LoadConfig() error {
	configPath := getConfigPath()
	log.Printf("[INFO] Loading config from: %s", configPath)

	data, err := os.ReadFile(configPath)
	if err != nil {
		log.Printf("[WARNING] Failed to read config file: %v", err)
		return fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		log.Printf("[ERROR] Failed to parse config: %v", err)
		return fmt.Errorf("failed to parse config: %w", err)
	}

	if err := validateConfig(&cfg); err != nil {
		log.Printf("[WARNING] Config validation warning: %v", err)
	}

	config = &cfg
	log.Printf("[INFO] Config loaded successfully")
	return nil
}

func validateConfig(cfg *Config) error {
	var warnings []string

	if cfg.Chroma.Host == "" {
		cfg.Chroma.Host = "localhost"
		warnings = append(warnings, "Chroma host not set, using default: localhost")
	}
	if cfg.Chroma.Port == 0 {
		cfg.Chroma.Port = 8000
		warnings = append(warnings, "Chroma port not set, using default: 8000")
	}
	if cfg.Chroma.Collection == "" {
		cfg.Chroma.Collection = "yichouchou_knowledge"
		warnings = append(warnings, "Chroma collection not set, using default: yichouchou_knowledge")
	}

	if len(warnings) > 0 {
		return fmt.Errorf(strings.Join(warnings, "; "))
	}
	return nil
}

func GetConfig() *Config {
	if config == nil {
		if err := LoadConfig(); err != nil {
			log.Printf("[ERROR] Failed to load config: %v", err)
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

func GetChromaConfig() *ChromaConfig {
	if cfg := GetConfig(); cfg != nil {
		return &cfg.Chroma
	}
	return nil
}
