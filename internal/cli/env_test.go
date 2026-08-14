package cli

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadEnvFileLoadsCredentialsWithoutOverridingEnvironment(t *testing.T) {
	const mapsKey = "GOOGLE_MAPS_API_KEY"
	const username = "DATAFORSEO_USERNAME"
	const openRouterKey = "OPENROUTER_API_KEY"
	restoreEnvironment(t, mapsKey)
	restoreEnvironment(t, username)
	restoreEnvironment(t, openRouterKey)
	if err := os.Setenv(mapsKey, "from-environment"); err != nil {
		t.Fatal(err)
	}
	if err := os.Unsetenv(username); err != nil {
		t.Fatal(err)
	}
	if err := os.Unsetenv(openRouterKey); err != nil {
		t.Fatal(err)
	}

	target := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(target, []byte("GOOGLE_MAPS_API_KEY=from-file\nDATAFORSEO_USERNAME='api user'\nOPENROUTER_API_KEY=router-key\nIGNORED_SECRET=nope\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := loadEnvFile(target); err != nil {
		t.Fatal(err)
	}
	if got := os.Getenv(mapsKey); got != "from-environment" {
		t.Fatalf("GOOGLE_MAPS_API_KEY = %q, want environment value", got)
	}
	if got := os.Getenv(username); got != "api user" {
		t.Fatalf("DATAFORSEO_USERNAME = %q, want file value", got)
	}
	if got := os.Getenv(openRouterKey); got != "router-key" {
		t.Fatalf("OPENROUTER_API_KEY = %q, want file value", got)
	}
}

func TestRequestedProviderCredentialsFailBeforeCrawl(t *testing.T) {
	t.Setenv("GOOGLE_MAPS_API_KEY", "")
	t.Setenv("DATAFORSEO_USERNAME", "user")
	t.Setenv("DATAFORSEO_PASSWORD", "password")
	opts := auditOptions{limit: 1, timeout: time.Second}
	err := opts.run(context.Background(), io.Discard, io.Discard, "place-1")
	if err == nil || !strings.Contains(err.Error(), "GOOGLE_MAPS_API_KEY") {
		t.Fatalf("run error = %v, want missing GOOGLE_MAPS_API_KEY", err)
	}
}

func TestFullAuditRequiresOpenRouterCredentialBeforeCrawl(t *testing.T) {
	t.Setenv("GOOGLE_MAPS_API_KEY", "maps-key")
	t.Setenv("DATAFORSEO_USERNAME", "user")
	t.Setenv("DATAFORSEO_PASSWORD", "password")
	t.Setenv("OPENROUTER_API_KEY", "")
	opts := auditOptions{limit: 1, timeout: time.Second, steps: "all"}
	err := opts.run(context.Background(), io.Discard, io.Discard, "place-1")
	if err == nil || !strings.Contains(err.Error(), "OPENROUTER_API_KEY") {
		t.Fatalf("run error = %v, want missing OPENROUTER_API_KEY", err)
	}
}

func TestBacklinksOnlyDoesNotRequireOpenRouterCredential(t *testing.T) {
	t.Setenv("GOOGLE_MAPS_API_KEY", "maps-key")
	t.Setenv("DATAFORSEO_USERNAME", "user")
	t.Setenv("DATAFORSEO_PASSWORD", "password")
	t.Setenv("OPENROUTER_API_KEY", "")
	if err := (auditOptions{steps: "backlinks"}).validateProviderCredentials(); err != nil {
		t.Fatal(err)
	}
}

func TestWebsiteOnlyRequiresOnlyGoogleMapsCredential(t *testing.T) {
	t.Setenv("GOOGLE_MAPS_API_KEY", "maps-key")
	t.Setenv("DATAFORSEO_USERNAME", "")
	t.Setenv("DATAFORSEO_PASSWORD", "")
	t.Setenv("OPENROUTER_API_KEY", "")
	if err := (auditOptions{steps: "website"}).validateProviderCredentials(); err != nil {
		t.Fatal(err)
	}
}

func restoreEnvironment(t *testing.T, key string) {
	t.Helper()
	value, exists := os.LookupEnv(key)
	t.Cleanup(func() {
		if exists {
			if err := os.Setenv(key, value); err != nil {
				t.Errorf("restore %s: %v", key, err)
			}
			return
		}
		if err := os.Unsetenv(key); err != nil {
			t.Errorf("unset %s: %v", key, err)
		}
	})
}
