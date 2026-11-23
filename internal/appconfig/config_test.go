package appconfig

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadAndStrategyFor(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app-strategies.json")
	data := []byte(`{
  "default": "virtual-backspace-commit",
  "apps": {
    "chromium": "surrounding-delete-commit",
    "ignored": "not-a-strategy"
  }
}`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}

	config, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := config.StrategyFor("chromium"); got != StrategySurroundingDeleteCommit {
		t.Fatalf("chromium strategy = %q", got)
	}
	for _, identity := range []string{"", "kitty", "ignored"} {
		if got := config.StrategyFor(identity); got != StrategyVirtualBackspaceCommit {
			t.Errorf("strategy for %q = %q", identity, got)
		}
	}
}

func TestUnknownDefaultFallsBack(t *testing.T) {
	config := Config{Default: "unknown", Apps: map[string]Strategy{"chromium": "unknown"}}
	if got := config.StrategyFor("chromium"); got != StrategyVirtualBackspaceCommit {
		t.Fatalf("unknown strategy = %q", got)
	}
}

func TestLoadMalformedJSONReturnsDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app-strategies.json")
	if err := os.WriteFile(path, []byte(`{"apps":`), 0o600); err != nil {
		t.Fatal(err)
	}

	config, err := Load(path)
	if err == nil {
		t.Fatal("malformed JSON should return an error")
	}
	if got := config.StrategyFor("chromium"); got != StrategyVirtualBackspaceCommit {
		t.Fatalf("malformed JSON fallback = %q", got)
	}
}

func TestLoadRejectsTrailingJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app-strategies.json")
	if err := os.WriteFile(path, []byte(`{} {}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("trailing JSON should return an error")
	}
}
