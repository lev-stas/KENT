package config

import (
	"os"
	"path/filepath"
	"testing"
)

// The Helm chart renders these keys into config.yaml; a rename on either side
// would silently leave the filters unconfigured.
func TestLoadReadsKubernetesFilterKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	content := `
kubernetes:
  include_namespaces: ["prod"]
  exclude_namespaces: ["kube-system"]
  include_event_types: ["Warning"]
  include_kinds: ["Pod", "Node"]
  include_reasons: "^(Failed|BackOff)"
  exclude_reasons: "^Started$"
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	t.Setenv("CONFIG_PATH", path)

	var cfg Config
	if err := Load(&cfg); err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	assertList(t, "include_namespaces", cfg.Kubernetes.IncludeNamespaces, []string{"prod"})
	assertList(t, "exclude_namespaces", cfg.Kubernetes.ExcludeNamespaces, []string{"kube-system"})
	assertList(t, "include_event_types", cfg.Kubernetes.IncludeEventTypes, []string{"Warning"})
	assertList(t, "include_kinds", cfg.Kubernetes.IncludeKinds, []string{"Pod", "Node"})

	if cfg.Kubernetes.IncludeReasons != "^(Failed|BackOff)" {
		t.Fatalf("include_reasons = %q", cfg.Kubernetes.IncludeReasons)
	}
	if cfg.Kubernetes.ExcludeReasons != "^Started$" {
		t.Fatalf("exclude_reasons = %q", cfg.Kubernetes.ExcludeReasons)
	}
}

// The chart's documented escape hatch for overriding config is extraEnv, so
// the env tags carry the same contract as the yaml ones.
func TestLoadReadsKubernetesFilterEnvVars(t *testing.T) {
	t.Setenv("CONFIG_PATH", filepath.Join(t.TempDir(), "absent.yaml"))
	t.Setenv("K8S_INCLUDE_NAMESPACES", "prod")
	t.Setenv("K8S_EXCLUDE_NAMESPACES", "kube-system")
	t.Setenv("K8S_INCLUDE_EVENT_TYPES", "Warning")
	t.Setenv("K8S_INCLUDE_KINDS", "Pod,Node")
	t.Setenv("K8S_INCLUDE_REASONS", "^(Failed|BackOff)")
	t.Setenv("K8S_EXCLUDE_REASONS", "^Started$")

	var cfg Config
	if err := Load(&cfg); err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	assertList(t, "K8S_INCLUDE_NAMESPACES", cfg.Kubernetes.IncludeNamespaces, []string{"prod"})
	assertList(t, "K8S_EXCLUDE_NAMESPACES", cfg.Kubernetes.ExcludeNamespaces, []string{"kube-system"})
	assertList(t, "K8S_INCLUDE_EVENT_TYPES", cfg.Kubernetes.IncludeEventTypes, []string{"Warning"})
	assertList(t, "K8S_INCLUDE_KINDS", cfg.Kubernetes.IncludeKinds, []string{"Pod", "Node"})

	if cfg.Kubernetes.IncludeReasons != "^(Failed|BackOff)" {
		t.Fatalf("K8S_INCLUDE_REASONS = %q", cfg.Kubernetes.IncludeReasons)
	}
	if cfg.Kubernetes.ExcludeReasons != "^Started$" {
		t.Fatalf("K8S_EXCLUDE_REASONS = %q", cfg.Kubernetes.ExcludeReasons)
	}
}

func assertList(t *testing.T, key string, got, want []string) {
	t.Helper()

	if len(got) != len(want) {
		t.Fatalf("%s = %v, want %v", key, got, want)
	}

	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("%s = %v, want %v", key, got, want)
		}
	}
}
