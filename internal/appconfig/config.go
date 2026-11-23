package appconfig

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type Strategy string

const (
	StrategyVirtualBackspaceCommit  Strategy = "virtual-backspace-commit"
	StrategySurroundingDeleteCommit Strategy = "surrounding-delete-commit"
)

type Config struct {
	Default Strategy            `json:"default"`
	Apps    map[string]Strategy `json:"apps"`
}

func Defaults() Config {
	return Config{
		Default: StrategyVirtualBackspaceCommit,
		Apps:    map[string]Strategy{},
	}
}

func DefaultPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "gokey", "app-strategies.json"), nil
}

func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Defaults(), err
	}

	var raw Config
	if err := json.Unmarshal(data, &raw); err != nil {
		return Defaults(), err
	}

	config := Defaults()
	if isKnown(raw.Default) {
		config.Default = raw.Default
	}
	for identity, strategy := range raw.Apps {
		if identity != "" && isKnown(strategy) {
			config.Apps[identity] = strategy
		}
	}
	return config, nil
}

func (c Config) StrategyFor(identity string) Strategy {
	if strategy, ok := c.Apps[identity]; ok && isKnown(strategy) {
		return strategy
	}
	if isKnown(c.Default) {
		return c.Default
	}
	return StrategyVirtualBackspaceCommit
}

func isKnown(strategy Strategy) bool {
	switch strategy {
	case StrategyVirtualBackspaceCommit, StrategySurroundingDeleteCommit:
		return true
	default:
		return false
	}
}
