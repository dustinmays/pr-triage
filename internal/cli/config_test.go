package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/dustinmays/pr-triage/internal/config"
)

func TestConfigShowPrintsMergedConfig(t *testing.T) {
	tmpDir := t.TempDir()
	// Write a PARTIAL config: only base_ref + a custom routine model, no
	// signal_tiers / routing. `config show` must still print the full merged
	// tables from defaults.
	cfgDir := filepath.Join(tmpDir, ".pr-triage")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	partial := "base_ref: chunk/**\nmodel: claude-opus-4-8\n"
	if err := os.WriteFile(filepath.Join(cfgDir, "config.yaml"), []byte(partial), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"config", "show", "--repo-dir", tmpDir})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("Execute() error: %v", err)
	}

	// The printed YAML must parse and contain both the override and the merged defaults.
	var got config.Config
	if err := yaml.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("output is not valid YAML: %v\n---\n%s", err, buf.String())
	}
	if got.BaseRef != "chunk/**" {
		t.Errorf("base_ref = %q, want chunk/**", got.BaseRef)
	}
	if len(got.Routing) == 0 {
		t.Errorf("routing table missing from effective config output")
	}
	if _, ok := got.Routing["routine"]; !ok {
		t.Errorf("routing.routine missing from effective config output")
	}
	if len(got.SignalTiers.Rules) == 0 && got.SignalTiers.DefaultTier == "" {
		t.Errorf("signal_tiers missing from effective config output: %+v", got.SignalTiers)
	}
}
