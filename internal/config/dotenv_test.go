package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseDotEnvLine(t *testing.T) {
	tests := []struct {
		name  string
		line  string
		key   string
		value string
		fail  bool
	}{
		{"plain pair", "PORT=4000", "PORT", "4000", false},
		{"spaces around", "  PORT = 4000  ", "PORT", "4000", false},
		{"double quoted", "CALLBACK_URL=\"http://localhost:5000\"", "CALLBACK_URL", "http://localhost:5000", false},
		{"single quoted", "CALLBACK_SECRET='s3cret'", "CALLBACK_SECRET", "s3cret", false},
		{"export prefix", "export DATABASE_URL=postgres://db", "DATABASE_URL", "postgres://db", false},
		{"comment", "# PORT=4000", "", "", false},
		{"empty line", "", "", "", false},
		{"missing equals", "NOT_A_PAIR", "", "", true},
		{"empty key", "=value", "", "", true},
		{"windows line ending", "PORT=4000\r", "PORT", "4000", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key, value, err := parseDotEnvLine(tt.line)
			if tt.fail {
				if err == nil {
					t.Fatalf("expected error for %q, got key=%q value=%q", tt.line, key, value)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error for %q: %v", tt.line, err)
			}
			if key != tt.key || value != tt.value {
				t.Fatalf("got key=%q value=%q, want %q=%q", key, value, tt.key, tt.value)
			}
		})
	}
}

func TestLoadDotEnvSetsUnsetAndKeepsExisting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte("PORT=4000\nEMAIL=a@b.com\n"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	if err := os.Setenv("PORT", "9999"); err != nil {
		t.Fatalf("set PORT: %v", err)
	}
	t.Cleanup(func() { _ = os.Unsetenv("PORT") })

	t.Cleanup(func() { _ = os.Unsetenv("EMAIL") })

	if err := os.Setenv("PORT", "9999"); err != nil {
		t.Fatalf("set PORT: %v", err)
	}

	if err := loadDotEnv(path); err != nil {
		t.Fatalf("loadDotEnv: %v", err)
	}

	if got := os.Getenv("PORT"); got != "9999" {
		t.Errorf("PORT = %q, want existing environment to win with 9999", got)
	}
	if got := os.Getenv("EMAIL"); got != "a@b.com" {
		t.Errorf("EMAIL = %q, want unset variable filled from file", got)
	}
}

func TestLoadDotEnvMissingFileIsFine(t *testing.T) {
	if err := loadDotEnv(filepath.Join(t.TempDir(), "does_not_exist.env")); err != nil {
		t.Fatalf("missing file should not error, got %v", err)
	}
}

func TestParseDotEnvFileValidatesEntries(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	broken := strings.Join([]string{
		"ALPHA=1",
		"BROKEN LINE",
	}, "\n")
	if err := os.WriteFile(path, []byte(broken), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	_, err := parseDotEnvFile(path)
	if err == nil {
		t.Fatal("expected error for malformed line")
	}
	if !strings.Contains(err.Error(), "line 2") {
		t.Errorf("error %v should mention line 2", err)
	}
}
