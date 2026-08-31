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
		IncludeEventTypes []string `yaml:"include_event_types" env:"K8S_INCLUDE_EVENT_TYPES" env-separator:","`
		IncludeKinds      []string `yaml:"include_kinds" env:"K8S_INCLUDE_KINDS" env-separator:","`
		IncludeReasons    string   `yaml:"include_reasons" env:"K8S_INCLUDE_REASONS"`
		ExcludeReasons    string   `yaml:"exclude_reasons" env:"K8S_EXCLUDE_REASONS"`
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
		QueueSize    int               `yaml:"queue_size" env:"VL_QUEUE_SIZE"`
		Headers      map[string]string `yaml:"headers"`
		Auth         struct {
			BasicUsername string `yaml:"basic_username" env:"VL_AUTH_BASIC_USERNAME"`
			BasicPassword string `yaml:"basic_password" env:"VL_AUTH_BASIC_PASSWORD"`
			BearerToken   string `yaml:"bearer_token" env:"VL_AUTH_BEARER_TOKEN"`
		} `yaml:"auth"`
		TLS struct {
			InsecureSkipVerify bool   `yaml:"insecure_skip_verify" env:"VL_TLS_INSECURE_SKIP_VERIFY"`
			CAFile             string `yaml:"ca_file" env:"VL_TLS_CA_FILE"`
			CertFile           string `yaml:"cert_file" env:"VL_TLS_CERT_FILE"`
			KeyFile            string `yaml:"key_file" env:"VL_TLS_KEY_FILE"`
			ServerName         string `yaml:"server_name" env:"VL_TLS_SERVER_NAME"`
		} `yaml:"tls"`
	} `yaml:"victoria_logs"`
	Stdout struct {
		Enabled bool `yaml:"enabled" env:"STDOUT_ENABLED"`
	} `yaml:"stdout"`
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
