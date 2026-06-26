package proxy

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"jacpro/internal/buildinfo"
)

const Version = "1.0.0"
const defaultConfigFile = "config.json"

type Settings struct {
	BaseURL           string `json:"base_url"`
	APIKey            string `json:"apikey"`
	Version           string `json:"version"`
	StripTrailingYear bool   `json:"strip_trailing_year"`
	MergeV1           bool   `json:"merge_v1"`
	EnrichTitles      bool   `json:"enrich_titles"`
	SkipCatFilter     bool   `json:"skip_cat_filter"`
	RequestTimeout    int    `json:"request_timeout"`
	LogLevel          string `json:"log_level"`
	LogFile           string `json:"log_file"`
	Host              string `json:"host"`
	Port              int    `json:"port"`
}

type SettingsPatch struct {
	BaseURL           *string `json:"base_url"`
	APIKey            *string `json:"apikey"`
	Version           *string `json:"version"`
	StripTrailingYear *bool   `json:"strip_trailing_year"`
	MergeV1           *bool   `json:"merge_v1"`
	EnrichTitles      *bool   `json:"enrich_titles"`
	SkipCatFilter     *bool   `json:"skip_cat_filter"`
	RequestTimeout    *int    `json:"request_timeout"`
	LogLevel          *string `json:"log_level"`
	LogFile           *string `json:"log_file"`
	Host              *string `json:"host"`
	Port              *int    `json:"port"`
}

type SettingsStore struct {
	mu   sync.RWMutex
	path string
	s    Settings
}

type ConfigResolution struct {
	Path     string
	Source   string
	Found    bool
	Warnings []string
}

func ResolveConfigPath() ConfigResolution {
	cwd, err := os.Getwd()
	if err != nil {
		cwd = "."
	}
	executable, err := os.Executable()
	if err != nil {
		executable = ""
	}
	return resolveConfigPath(cwd, executable, os.Getenv("JACPRO_CONFIG"), fileExists)
}

func resolveConfigPath(cwd, executable, envPath string, exists func(string) bool) ConfigResolution {
	var warnings []string
	cwdPath := absPath(filepath.Join(cwd, defaultConfigFile))
	envPath = strings.TrimSpace(envPath)
	if envPath != "" {
		envPath = absPath(envPath)
	}

	binPath := ""
	if executable != "" {
		binPath = absPath(filepath.Join(filepath.Dir(executable), defaultConfigFile))
	}

	type candidate struct {
		path   string
		source string
	}
	candidates := []candidate{{path: cwdPath, source: "working directory"}}
	if envPath != "" {
		candidates = append(candidates, candidate{path: envPath, source: "JACPRO_CONFIG"})
	}
	if binPath != "" {
		candidates = append(candidates, candidate{path: binPath, source: "binary directory"})
	}

	seen := map[string]bool{}
	for _, item := range candidates {
		if item.path == "" || seen[item.path] {
			continue
		}
		seen[item.path] = true
		if exists(item.path) {
			return ConfigResolution{
				Path:     item.path,
				Source:   item.source,
				Found:    true,
				Warnings: warnings,
			}
		}
	}

	createPath := cwdPath
	source := "working directory"
	if envPath != "" {
		createPath = envPath
		source = "JACPRO_CONFIG"
	}
	return ConfigResolution{
		Path:     createPath,
		Source:   source,
		Found:    false,
		Warnings: warnings,
	}
}

func absPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	return abs
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func NewSettingsStore(path string) (*SettingsStore, []string, error) {
	settings := defaultSettingsFromEnv()
	var warnings []string
	if path != "" {
		patch, loaded, readWarnings, err := readSettingsPatch(path)
		warnings = append(warnings, readWarnings...)
		if err != nil {
			return nil, warnings, err
		}
		if loaded {
			applyPatch(&settings, patch)
		}
	}
	settings.Normalize()
	if err := settings.Validate(); err != nil {
		warnings = append(warnings, fmt.Sprintf("settings in %s are invalid (%v); recreating defaults", path, err))
		if path != "" {
			backupPath, backupErr := backupSettingsFile(path)
			if backupErr != nil {
				warnings = append(warnings, fmt.Sprintf("failed to back up invalid settings %s: %v", path, backupErr))
			} else if backupPath != "" {
				warnings = append(warnings, fmt.Sprintf("invalid settings moved to %s", backupPath))
			}
		}
		settings = defaultSettingsFromEnv()
		settings.Normalize()
		if err := settings.Validate(); err != nil {
			return nil, warnings, err
		}
	}
	if path != "" {
		if err := settings.Save(path); err != nil {
			return nil, warnings, err
		}
	}
	return &SettingsStore{path: path, s: settings}, warnings, nil
}

func defaultSettingsFromEnv() Settings {
	return Settings{
		BaseURL:           envString("JACRED_BASE_URL", "http://127.0.0.1:9117"),
		APIKey:            envString("JACRED_APIKEY", ""),
		Version:           buildinfo.Version,
		StripTrailingYear: envBool("JACRED_STRIP_YEAR", false),
		MergeV1:           envBool("JACRED_MERGE_V1", true),
		EnrichTitles:      envBool("JACRED_ENRICH_TITLES", true),
		SkipCatFilter:     envBool("JACRED_SKIP_CAT_FILTER", false),
		RequestTimeout:    envInt("JACRED_TIMEOUT", 20),
		LogLevel:          strings.ToUpper(envString("LOG_LEVEL", "INFO")),
		LogFile:           envString("LOG_FILE", "/tmp/jacpro.log"),
		Host:              envString("HOST", "0.0.0.0"),
		Port:              envInt("PORT", 5002),
	}
}

func readSettingsPatch(path string) (SettingsPatch, bool, []string, error) {
	var patch SettingsPatch
	var warnings []string
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return patch, false, warnings, nil
		}
		return patch, false, warnings, fmt.Errorf("read settings %s: %w", path, err)
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return patch, false, warnings, nil
	}
	if err := json.Unmarshal(data, &patch); err != nil {
		warnings = append(warnings, fmt.Sprintf("settings JSON in %s is invalid (%v); recreating defaults", path, err))
		backupPath, backupErr := backupSettingsFile(path)
		if backupErr != nil {
			warnings = append(warnings, fmt.Sprintf("failed to back up invalid JSON %s: %v", path, backupErr))
		} else if backupPath != "" {
			warnings = append(warnings, fmt.Sprintf("invalid JSON moved to %s", backupPath))
		}
		return patch, false, warnings, nil
	}
	return patch, true, warnings, nil
}

func backupSettingsFile(path string) (string, error) {
	if path == "" {
		return "", nil
	}
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
		return "", err
	}
	backupPath := fmt.Sprintf("%s.bad-%s", path, time.Now().Format("20060102-150405"))
	if err := os.Rename(path, backupPath); err != nil {
		return "", err
	}
	return backupPath, nil
}

func (s Settings) Save(path string) error {
	if path == "" {
		return nil
	}
	dir := filepath.Dir(path)
	if dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create settings dir %s: %w", dir, err)
		}
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("write settings %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("replace settings %s: %w", path, err)
	}
	return nil
}

func (s *Settings) Normalize() {
	s.BaseURL = strings.TrimSpace(s.BaseURL)
	s.BaseURL = strings.TrimRight(s.BaseURL, "/")
	s.APIKey = strings.TrimSpace(s.APIKey)
	s.Version = strings.TrimSpace(s.Version)
	if s.Version == "" {
		s.Version = buildinfo.Version
	}
	s.LogLevel = strings.ToUpper(strings.TrimSpace(s.LogLevel))
	if s.LogLevel == "" {
		s.LogLevel = "INFO"
	}
	s.LogFile = strings.TrimSpace(s.LogFile)
	s.Host = strings.TrimSpace(s.Host)
	if s.Host == "" {
		s.Host = "0.0.0.0"
	}
	if s.RequestTimeout <= 0 {
		s.RequestTimeout = 20
	}
}

func (s Settings) Validate() error {
	if s.BaseURL == "" {
		return errors.New("base_url is required")
	}
	parsed, err := url.Parse(s.BaseURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return fmt.Errorf("base_url must be an absolute HTTP URL")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("base_url scheme must be http or https")
	}
	if s.RequestTimeout <= 0 {
		return fmt.Errorf("request_timeout must be positive")
	}
	if s.Port <= 0 || s.Port > 65535 {
		return fmt.Errorf("port must be between 1 and 65535")
	}
	if _, ok := parseLogLevel(s.LogLevel); !ok {
		return fmt.Errorf("log_level must be DEBUG, INFO, WARNING, ERROR, or CRITICAL")
	}
	return nil
}

func (s *SettingsStore) Get() Settings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.s
}

func (s *SettingsStore) Replace(next Settings) error {
	next.Normalize()
	if err := next.Validate(); err != nil {
		return err
	}
	if err := next.Save(s.path); err != nil {
		return err
	}
	s.mu.Lock()
	s.s = next
	s.mu.Unlock()
	return nil
}

func (s *SettingsStore) Update(patch SettingsPatch) (Settings, error) {
	current := s.Get()
	applyPatch(&current, patch)
	if err := s.Replace(current); err != nil {
		return Settings{}, err
	}
	return current, nil
}

func applyPatch(s *Settings, patch SettingsPatch) {
	if patch.BaseURL != nil {
		s.BaseURL = *patch.BaseURL
	}
	if patch.APIKey != nil {
		s.APIKey = *patch.APIKey
	}
	if patch.Version != nil {
		s.Version = *patch.Version
	}
	if patch.StripTrailingYear != nil {
		s.StripTrailingYear = *patch.StripTrailingYear
	}
	if patch.MergeV1 != nil {
		s.MergeV1 = *patch.MergeV1
	}
	if patch.EnrichTitles != nil {
		s.EnrichTitles = *patch.EnrichTitles
	}
	if patch.SkipCatFilter != nil {
		s.SkipCatFilter = *patch.SkipCatFilter
	}
	if patch.RequestTimeout != nil {
		s.RequestTimeout = *patch.RequestTimeout
	}
	if patch.LogLevel != nil {
		s.LogLevel = *patch.LogLevel
	}
	if patch.LogFile != nil {
		s.LogFile = *patch.LogFile
	}
	if patch.Host != nil {
		s.Host = *patch.Host
	}
	if patch.Port != nil {
		s.Port = *patch.Port
	}
}

func envString(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func envBool(name string, fallback bool) bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv(name)))
	if value == "" {
		return fallback
	}
	return value == "1" || value == "true" || value == "yes" || value == "on"
}

func envInt(name string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return value
}
