// Copyright 2025 Stas Levchenko
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//     http://www.apache.org/licenses/LICENSE-2.0

package config

import (
	"os"
	"time"

	"github.com/ilyakaznacheev/cleanenv"
)

type Config struct {
	Kubernetes struct {
		IncludeNamespaces []string `yaml:"include_namespaces" env:"K8S_INCLUDE_NAMESPACES" env-separator:","`
		ExcludeNamespaces []string `yaml:"exclude_namespaces" env:"K8S_EXCLUDE_NAMESPACES" env-separator:","`
	} `yaml:"kubernetes"`
	VictoriaLogs struct {
		Enabled      bool              `yaml:"enabled" env:"VL_ENABLED"`
		Endpoint     string            `yaml:"endpoint" env:"VL_ENDPOINT"`
		ClusterID    string            `yaml:"cluster_id" env:"VL_CLUSTER_ID"`
		BatchSize    int               `yaml:"batch_size" env:"VL_BATCH_SIZE"`
		FlushTime    time.Duration     `yaml:"flush_time" env:"VL_FLUSH_TIME"`
		ExtraFields  map[string]string `yaml:"extra_fields" env-prefix:"VL_EXTRA_"`
		Timeout      time.Duration     `yaml:"timeout" env:"VL_TIMEOUT"`
		AccountID    string            `yaml:"account_id" env:"VL_ACCOUNT_ID"`
		ProjectID    string            `yaml:"project_id" env:"VL_PROJECT_ID"`
		StreamFields []string          `yaml:"stream_fields" env:"VL_STREAM_FIELDS" env-separator:","`
	} `yaml:"victoria_logs"`
	HealthConfig struct {
		Port int `yaml:"port" env:"HEALTH_PORT" env-default:"8080"`
	} `yaml:"health"`
	Logger struct {
		Level string `yaml:"level" env:"LOG_LEVEL" env-default:"info"`
	} `yaml:"logger"`
}

func Load(cfg *Config) error {
	path := os.Getenv("CONFIG_PATH")
	if path == "" {
		path = "config.yaml"
	}

	if _, err := os.Stat(path); err == nil {
		if err := cleanenv.ReadConfig(path, cfg); err != nil {
			return err
		}
	} else {
		if err := cleanenv.ReadEnv(cfg); err != nil {
			return err
		}
	}

	return nil
}
