package agentsync

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRenderClaude(t *testing.T) {
	s := &Source{
		Name:        "reviewer",
		Description: "Reviews things.",
		Tools:       []string{"Bash", "Read"},
		Model:       "",
		Mode:        "all",
		Prompt:      "# Reviewer\n\nDo the review.\n",
	}
	out := RenderClaude(s)

	if !strings.Contains(out, "name: reviewer\n") {
		t.Errorf("missing name line, got:\n%s", out)
	}
	if !strings.Contains(out, "description: Reviews things.\n") {
		t.Errorf("missing description line, got:\n%s", out)
	}
	if !strings.Contains(out, "tools: Bash, Read\n") {
		t.Errorf("missing/incorrect tools line, got:\n%s", out)
	}
	if strings.Contains(out, "model:") {
		t.Errorf("unexpected model line, got:\n%s", out)
	}
	if !strings.Contains(out, "Do the review.") {
		t.Errorf("missing prompt body, got:\n%s", out)
	}
	if !strings.HasSuffix(out, "\n") || strings.HasSuffix(out, "\n\n") {
		t.Errorf("expected exactly one trailing newline, got:\n%q", out)
	}
}

func TestRenderClaude_WithModel(t *testing.T) {
	s := &Source{
		Name:        "reviewer",
		Description: "Reviews things.",
		Tools:       []string{"Bash"},
		Model:       "claude-opus-4",
		Mode:        "all",
		Prompt:      "Body.\n",
	}
	out := RenderClaude(s)
	if !strings.Contains(out, "model: claude-opus-4\n") {
		t.Errorf("expected model line, got:\n%s", out)
	}
}

func TestRenderOpenCode(t *testing.T) {
	s := &Source{
		Name:        "reviewer",
		Description: "Reviews things.",
		Tools:       []string{"Bash", "Read"},
		Model:       "",
		Mode:        "all",
		Prompt:      "Body text.\n",
	}
	out := RenderOpenCode(s)

	if !strings.Contains(out, "mode: all\n") {
		t.Errorf("expected mode: all, got:\n%s", out)
	}
	if !strings.Contains(out, "tools:\n  bash: true\n  read: true\n") {
		t.Errorf("expected sorted tools block, got:\n%s", out)
	}
	if !strings.Contains(out, "Body text.") {
		t.Errorf("missing prompt body, got:\n%s", out)
	}
	if strings.Contains(out, "model:") {
		t.Errorf("unexpected model line, got:\n%s", out)
	}
}

func TestSyncThenCheckClean(t *testing.T) {
	agentsDir := t.TempDir()
	root := t.TempDir()

	yamlContent := `name: sample
description: A sample agent.
tools: [Bash, Read]
model: ""
mode: all
prompt: |
  Hello.
`
	if err := os.WriteFile(filepath.Join(agentsDir, "sample.agent.yaml"), []byte(yamlContent), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := Sync(agentsDir, root); err != nil {
		t.Fatalf("Sync failed: %v", err)
	}

	drift, err := Check(agentsDir, root)
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}
	if len(drift) != 0 {
		t.Errorf("expected empty drift list, got: %v", drift)
	}
}

func TestCheckDetectsDrift(t *testing.T) {
	agentsDir := t.TempDir()
	root := t.TempDir()

	yamlContent := `name: sample
description: A sample agent.
tools: [Bash, Read]
model: ""
mode: all
prompt: |
  Hello.
`
	if err := os.WriteFile(filepath.Join(agentsDir, "sample.agent.yaml"), []byte(yamlContent), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := Sync(agentsDir, root); err != nil {
		t.Fatalf("Sync failed: %v", err)
	}

	mutated := filepath.Join(root, ".claude", "agents", "sample.md")
	if err := os.WriteFile(mutated, []byte("mutated content\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	drift, err := Check(agentsDir, root)
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}
	if len(drift) != 1 || drift[0] != filepath.Join(".claude", "agents", "sample.md") {
		t.Errorf("expected drift on the mutated file only, got: %v", drift)
	}
}

func TestLoadAllSortedDeterministic(t *testing.T) {
	agentsDir := t.TempDir()

	zebra := `name: zebra
description: Z agent.
tools: [Bash]
model: ""
mode: all
prompt: |
  Z body.
`
	apple := `name: apple
description: A agent.
tools: [Bash]
model: ""
mode: all
prompt: |
  A body.
`
	if err := os.WriteFile(filepath.Join(agentsDir, "zebra.agent.yaml"), []byte(zebra), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(agentsDir, "apple.agent.yaml"), []byte(apple), 0o644); err != nil {
		t.Fatal(err)
	}

	sources, err := LoadAll(agentsDir)
	if err != nil {
		t.Fatalf("LoadAll failed: %v", err)
	}
	if len(sources) != 2 {
		t.Fatalf("expected 2 sources, got %d", len(sources))
	}
	if sources[0].Name != "apple" || sources[1].Name != "zebra" {
		t.Errorf("expected sorted order [apple, zebra], got [%s, %s]", sources[0].Name, sources[1].Name)
	}
}
