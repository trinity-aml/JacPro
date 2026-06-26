package proxy

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveConfigPathPriority(t *testing.T) {
	root := t.TempDir()
	cwd := filepath.Join(root, "cwd")
	envDir := filepath.Join(root, "env")
	binDir := filepath.Join(root, "bin")
	for _, dir := range []string{cwd, envDir, binDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	envConfig := filepath.Join(envDir, defaultConfigFile)
	binConfig := filepath.Join(binDir, defaultConfigFile)
	writeTestFile(t, envConfig, "{}")
	writeTestFile(t, binConfig, "{}")

	exists := func(path string) bool {
		info, err := os.Stat(path)
		return err == nil && !info.IsDir()
	}

	resolved := resolveConfigPath(cwd, filepath.Join(binDir, "jacpro"), envConfig, exists)
	if resolved.Path != envConfig || resolved.Source != "JACPRO_CONFIG" || !resolved.Found {
		t.Fatalf("expected env config, got %#v", resolved)
	}

	cwdConfig := filepath.Join(cwd, defaultConfigFile)
	writeTestFile(t, cwdConfig, "{}")
	resolved = resolveConfigPath(cwd, filepath.Join(binDir, "jacpro"), envConfig, exists)
	if resolved.Path != cwdConfig || resolved.Source != "working directory" || !resolved.Found {
		t.Fatalf("expected cwd config to win, got %#v", resolved)
	}
}

func TestResolveConfigPathUsesBinaryAfterCwdAndEnv(t *testing.T) {
	root := t.TempDir()
	cwd := filepath.Join(root, "cwd")
	binDir := filepath.Join(root, "bin")
	if err := os.MkdirAll(cwd, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	binConfig := filepath.Join(binDir, defaultConfigFile)
	writeTestFile(t, binConfig, "{}")

	resolved := resolveConfigPath(cwd, filepath.Join(binDir, "jacpro"), "", fileExists)
	if resolved.Path != binConfig || resolved.Source != "binary directory" || !resolved.Found {
		t.Fatalf("expected binary config, got %#v", resolved)
	}
}

func TestResolveConfigPathCreatesAtEnvWhenNoFileExists(t *testing.T) {
	root := t.TempDir()
	cwd := filepath.Join(root, "cwd")
	envConfig := filepath.Join(root, "custom", "settings.json")
	if err := os.MkdirAll(cwd, 0o755); err != nil {
		t.Fatal(err)
	}

	resolved := resolveConfigPath(cwd, filepath.Join(root, "bin", "jacpro"), envConfig, func(string) bool { return false })
	if resolved.Path != envConfig || resolved.Source != "JACPRO_CONFIG" || resolved.Found {
		t.Fatalf("expected env creation target, got %#v", resolved)
	}
}

func TestNewSettingsStoreCreatesDefaultConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	store, warnings, err := NewSettingsStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %#v", warnings)
	}
	if !fileExists(path) {
		t.Fatalf("expected settings file to be created")
	}
	settings := store.Get()
	if settings.BaseURL == "" || settings.Port == 0 {
		t.Fatalf("unexpected defaults: %#v", settings)
	}
}

func TestNewSettingsStoreRecoversInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	writeTestFile(t, path, "{broken")

	store, warnings, err := NewSettingsStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) == 0 {
		t.Fatalf("expected warning for invalid JSON")
	}
	if store.Get().BaseURL != "http://127.0.0.1:9117" {
		t.Fatalf("expected default settings after invalid JSON, got %#v", store.Get())
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "{broken") {
		t.Fatalf("invalid JSON was not replaced: %s", data)
	}
	matches, err := filepath.Glob(path + ".bad-*")
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected one backup file, got %#v", matches)
	}
}

func TestNewSettingsStoreRecoversInvalidValues(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	writeTestFile(t, path, `{"base_url":"::::","port":70000}`)

	store, warnings, err := NewSettingsStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) == 0 {
		t.Fatalf("expected warning for invalid values")
	}
	if store.Get().Port != 5002 {
		t.Fatalf("expected default port after invalid config, got %#v", store.Get())
	}
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
