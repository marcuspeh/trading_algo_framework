package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/marcuspeh/trading-framework/config"
	"github.com/marcuspeh/trading-framework/job"
)

func writeFile(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "strategy.conf")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestFile_ReadsKeyValuePairs(t *testing.T) {
	path := writeFile(t, `
# a comment
strategy = "momentum"
max_positions = 5
starting_cash = 10000.5
dry_run = true
`)
	cfg, err := config.NewFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if v, err := cfg.Get("strategy"); err != nil || v != "momentum" {
		t.Fatalf("strategy=%q err=%v", v, err)
	}
	if v, err := cfg.GetInt("max_positions"); err != nil || v != 5 {
		t.Fatalf("max_positions=%d err=%v", v, err)
	}
	if v, err := cfg.GetFloat("starting_cash"); err != nil || v != 10000.5 {
		t.Fatalf("starting_cash=%f err=%v", v, err)
	}
	if v, err := cfg.GetBool("dry_run"); err != nil || !v {
		t.Fatalf("dry_run=%v err=%v", v, err)
	}
}

func TestFile_MissingKeyReturnsErrNotFound(t *testing.T) {
	cfg := config.NewMap(map[string]string{"a": "1"})
	if _, err := cfg.Get("missing"); err != job.ErrNotFound {
		t.Fatalf("err=%v want ErrNotFound", err)
	}
}

func TestFile_IgnoresCommentsAndBlankLines(t *testing.T) {
	path := writeFile(t, "# comment\n\n   \nkey = value\n")
	cfg, err := config.NewFile(path)
	if err != nil {
		t.Fatal(err)
	}
	keys := cfg.Keys()
	if len(keys) != 1 || keys[0] != "key" {
		t.Fatalf("keys=%v", keys)
	}
}

func TestFile_MalformedLineIsRejected(t *testing.T) {
	path := writeFile(t, "this line has no equals sign\n")
	if _, err := config.NewFile(path); err == nil {
		t.Fatal("expected error for malformed line")
	}
}

func TestFile_MissingFileIsError(t *testing.T) {
	if _, err := config.NewFile(filepath.Join(t.TempDir(), "nope.conf")); err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestFile_ReloadPicksUpChanges(t *testing.T) {
	path := writeFile(t, "key = one\n")
	cfg, err := config.NewFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if v, _ := cfg.Get("key"); v != "one" {
		t.Fatalf("key=%q", v)
	}
	if err := os.WriteFile(path, []byte("key = two\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := cfg.Reload(path); err != nil {
		t.Fatal(err)
	}
	if v, _ := cfg.Get("key"); v != "two" {
		t.Fatalf("key=%q want two", v)
	}
}


func TestFile_SatisfiesJobConfig(t *testing.T) {
	var _ job.Config = config.NewMap(nil)
}
