package config

import (
	"fmt"
	"os"
	"sync"

	"gopkg.in/yaml.v3"
)

const (
	defaultConfigFile = "config.yaml"
	hardcodedDefault  = "gemini-2.5-pro"
)

type Config struct {
	Models ModelsConfig `yaml:"models"`
}

type ModelsConfig struct {
	Default                    string `yaml:"default"`
	Orchestrator               string `yaml:"orchestrator"`
	RepoChecker                string `yaml:"repo_checker"`
	RepoCheckerSubAgents       string `yaml:"repo_checker_sub_agents"`
	PlatformChecker            string `yaml:"platform_checker"`
	PlatformCheckerSubAgents   string `yaml:"platform_checker_sub_agents"`
	Correlator                 string `yaml:"correlator"`
	Reporter                   string `yaml:"reporter"`
}

var (
	cfg     *Config
	cfgErr  error
	cfgOnce sync.Once
)

// Load reads and parses the config file. Missing file is not an error.
// Bad YAML is an error. Uses sync.Once for singleton behavior.
func Load() (*Config, error) {
	cfgOnce.Do(func() {
		cfg = &Config{}

		path := os.Getenv("AGENTS_CONFIG")
		if path == "" {
			path = defaultConfigFile
		}

		data, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				return // missing file is fine, use zero-value config
			}
			cfgErr = fmt.Errorf("reading config %s: %w", path, err)
			return
		}

		if err := yaml.Unmarshal(data, cfg); err != nil {
			cfgErr = fmt.Errorf("parsing config %s: %w", path, err)
		}
	})
	return cfg, cfgErr
}

// ModelForAgent returns the model name for the given agent, falling back
// to the default model, then to the hardcoded default.
func (c *Config) ModelForAgent(agent string) string {
	var specific string
	switch agent {
	case "orchestrator":
		specific = c.Models.Orchestrator
	case "repo_checker":
		specific = c.Models.RepoChecker
	case "repo_checker_sub_agents":
		specific = c.Models.RepoCheckerSubAgents
	case "platform_checker":
		specific = c.Models.PlatformChecker
	case "platform_checker_sub_agents":
		specific = c.Models.PlatformCheckerSubAgents
	case "correlator":
		specific = c.Models.Correlator
	case "reporter":
		specific = c.Models.Reporter
	}
	if specific != "" {
		return specific
	}
	if c.Models.Default != "" {
		return c.Models.Default
	}
	return hardcodedDefault
}

// ModelName is a convenience function that loads config and returns
// the model name for the given agent. Never fails — returns the
// hardcoded default on error.
func ModelName(agent string) string {
	c, err := Load()
	if err != nil {
		return hardcodedDefault
	}
	return c.ModelForAgent(agent)
}
