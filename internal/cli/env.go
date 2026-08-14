package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func (opts auditOptions) loadProviderCredentials() error {
	if opts.validateProviderCredentials() == nil {
		return nil
	}
	if err := loadEnvFile(".env"); err != nil {
		return err
	}
	if opts.validateProviderCredentials() == nil {
		return nil
	}
	configDir, err := os.UserConfigDir()
	if err != nil {
		return fmt.Errorf("find user config directory: %w", err)
	}
	if err := loadEnvFile(filepath.Join(configDir, "seoaudit", ".env")); err != nil {
		return err
	}
	return opts.validateProviderCredentials()
}
func (opts auditOptions) validateProviderCredentials() error {
	if strings.TrimSpace(os.Getenv("GOOGLE_MAPS_API_KEY")) == "" {
		return errors.New("GOOGLE_MAPS_API_KEY is required to resolve the Place ID")
	}
	usesDataForSEO := opts.steps == "" || opts.steps == "all" || opts.steps == "visibility" || opts.steps == "backlinks"
	if usesDataForSEO && (strings.TrimSpace(os.Getenv("DATAFORSEO_USERNAME")) == "" || strings.TrimSpace(os.Getenv("DATAFORSEO_PASSWORD")) == "") {
		return errors.New("DATAFORSEO_USERNAME and DATAFORSEO_PASSWORD are required for DataForSEO checks")
	}
	usesOpenRouter := opts.steps == "" || opts.steps == "all" || opts.steps == "visibility"
	if usesOpenRouter && strings.TrimSpace(os.Getenv("OPENROUTER_API_KEY")) == "" {
		return errors.New("OPENROUTER_API_KEY is required for local visibility research")
	}
	return nil
}

func loadEnvFile(filePath string) error {
	data, err := os.ReadFile(filePath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read credentials from %s: %w", filePath, err)
	}
	for lineNumber, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		key, value, ok := strings.Cut(line, "=")
		key = strings.TrimSpace(key)
		if !ok || !credentialName(key) {
			continue
		}
		if _, exists := os.LookupEnv(key); exists {
			continue
		}
		value, err = envValue(value)
		if err != nil {
			return fmt.Errorf("read credentials from %s line %d: %w", filePath, lineNumber+1, err)
		}
		if err := os.Setenv(key, value); err != nil {
			return fmt.Errorf("set %s from %s: %w", key, filePath, err)
		}
	}
	return nil
}

func credentialName(value string) bool {
	switch value {
	case "GOOGLE_MAPS_API_KEY", "DATAFORSEO_USERNAME", "DATAFORSEO_PASSWORD", "OPENROUTER_API_KEY":
		return true
	default:
		return false
	}
}

func envValue(value string) (string, error) {
	value = strings.TrimSpace(value)
	if len(value) < 2 {
		return value, nil
	}
	if value[0] == '\'' {
		if value[len(value)-1] != '\'' {
			return "", errors.New("unterminated single-quoted value")
		}
		return value[1 : len(value)-1], nil
	}
	if value[0] != '"' {
		return value, nil
	}
	parsed, err := strconv.Unquote(value)
	if err != nil {
		return "", fmt.Errorf("invalid double-quoted value: %w", err)
	}
	return parsed, nil
}
