// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	// DefaultAPIURL is the production Fleet Intelligence API root.
	DefaultAPIURL = "https://api.fleet-intelligence.nvidia.com"
	// EnvAPIURL overrides the configured API URL for the current process.
	EnvAPIURL = "NVFLEETINT_API_URL"
	// EnvServiceKey overrides the configured service key for the current process.
	EnvServiceKey = "NVFLEETINT_SERVICE_KEY"
	dirName       = "nvfleetint"
	fileName      = "config.yaml"
	fileMode      = 0o600
	dirMode       = 0o700
)

type Config struct {
	APIURL     string
	ServiceKey string
}

func Default() Config {
	return Config{APIURL: DefaultAPIURL}
}

func Path() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(homeDir) == "" {
		return "", errors.New("home directory is required")
	}

	return filepath.Join(homeDir, ".config", dirName, fileName), nil
}

func Load() (Config, error) {
	path, err := Path()
	if err != nil {
		return Config{}, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Default(), nil
		}
		return Config{}, err
	}

	cfg, err := parse(data)
	if err != nil {
		return Config{}, fmt.Errorf("read config: %w", err)
	}
	normalize(&cfg)

	return cfg, nil
}

// LoadWithEnv loads config from disk and overlays supported environment variables.
func LoadWithEnv() (Config, error) {
	cfg, err := Load()
	if err != nil {
		return Config{}, err
	}
	ApplyEnv(&cfg)
	return cfg, nil
}

// ApplyEnv overlays supported environment variables onto cfg.
func ApplyEnv(cfg *Config) {
	apiURL := strings.TrimSpace(os.Getenv(EnvAPIURL))
	if apiURL != "" {
		cfg.APIURL = apiURL
	}
	serviceKey := strings.TrimSpace(os.Getenv(EnvServiceKey))
	if serviceKey != "" {
		cfg.ServiceKey = serviceKey
	}
	normalize(cfg)
}

func Save(cfg Config) error {
	normalize(&cfg)

	path, err := Path()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), dirMode); err != nil {
		return err
	}
	if err := os.WriteFile(path, format(cfg), fileMode); err != nil {
		return err
	}

	return os.Chmod(path, fileMode)
}

func normalize(cfg *Config) {
	cfg.APIURL = strings.TrimSpace(cfg.APIURL)
	cfg.ServiceKey = strings.TrimSpace(cfg.ServiceKey)
	if cfg.APIURL == "" {
		cfg.APIURL = DefaultAPIURL
	}
}

func format(cfg Config) []byte {
	return []byte(fmt.Sprintf("api_url: %s\nservice_key: %s\n", quote(cfg.APIURL), quote(cfg.ServiceKey)))
}

func quote(value string) string {
	return strconv.Quote(value)
}

func parse(data []byte) (Config, error) {
	var cfg Config

	lines := strings.Split(string(data), "\n")
	for i, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		key, value, ok := strings.Cut(line, ":")
		if !ok {
			return Config{}, fmt.Errorf("line %d: expected key/value pair", i+1)
		}

		key = strings.TrimSpace(key)
		parsedValue, err := parseValue(strings.TrimSpace(value))
		if err != nil {
			return Config{}, fmt.Errorf("line %d: %w", i+1, err)
		}

		switch key {
		case "api_url":
			cfg.APIURL = parsedValue
		case "service_key":
			cfg.ServiceKey = parsedValue
		}
	}

	return cfg, nil
}

func parseValue(value string) (string, error) {
	if value == "" {
		return "", nil
	}
	if strings.HasPrefix(value, "\"") {
		parsedValue, err := strconv.Unquote(value)
		if err != nil {
			return "", err
		}
		return parsedValue, nil
	}

	return value, nil
}
