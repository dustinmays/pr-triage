// Package agentsync renders a single neutral agent definition
// (agents/<name>.agent.yaml) into the various tool-specific formats that
// Claude Code and OpenCode expect. Adding a new target tool means adding a
// new render function and wiring it into Targets; the neutral source never
// changes.
package agentsync

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Source is one agent's tool-agnostic definition, loaded from
// agents/<name>.agent.yaml.
type Source struct {
	Name        string   `yaml:"name"`
	Description string   `yaml:"description"`
	Tools       []string `yaml:"tools"`
	Model       string   `yaml:"model"`
	Mode        string   `yaml:"mode"`
	Prompt      string   `yaml:"prompt"`
}

// Target is a generated output file for one agent.
type Target struct {
	Path    string // repo-relative, e.g. ".claude/agents/review-agent.md"
	Content string
}

// claudeToolNames are the canonical Claude Code tool names, in the fixed
// order used for the comma-separated frontmatter list.
var claudeToOpenCode = map[string]string{
	"Bash":      "bash",
	"Read":      "read",
	"Edit":      "edit",
	"Write":     "write",
	"Glob":      "glob",
	"Grep":      "grep",
	"WebFetch":  "webfetch",
	"WebSearch": "websearch",
	"Task":      "task",
	"List":      "list",
	"TodoWrite": "todowrite",
}

// LoadSource reads and unmarshals one agent.yaml file.
func LoadSource(path string) (*Source, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("agentsync: read %s: %w", path, err)
	}
	var s Source
	if err := yaml.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("agentsync: parse %s: %w", path, err)
	}
	if s.Name == "" {
		return nil, fmt.Errorf("agentsync: %s: name is required", path)
	}
	if s.Mode == "" {
		s.Mode = "all"
	}
	return &s, nil
}

// LoadAll globs <dir>/*.agent.yaml, loads each, and returns them sorted by
// Name for deterministic output.
func LoadAll(dir string) ([]*Source, error) {
	matches, err := filepath.Glob(filepath.Join(dir, "*.agent.yaml"))
	if err != nil {
		return nil, fmt.Errorf("agentsync: glob %s: %w", dir, err)
	}
	sources := make([]*Source, 0, len(matches))
	for _, m := range matches {
		s, err := LoadSource(m)
		if err != nil {
			return nil, err
		}
		sources = append(sources, s)
	}
	sort.Slice(sources, func(i, j int) bool { return sources[i].Name < sources[j].Name })
	return sources, nil
}

// withSingleTrailingNewline trims trailing whitespace/newlines and appends
// exactly one newline.
func withSingleTrailingNewline(s string) string {
	return strings.TrimRight(s, "\n") + "\n"
}

// RenderClaude renders a Source into Claude Code agent-file content.
func RenderClaude(s *Source) string {
	var b strings.Builder
	b.WriteString("---\n")
	fmt.Fprintf(&b, "name: %s\n", s.Name)
	fmt.Fprintf(&b, "description: %s\n", s.Description)
	if len(s.Tools) > 0 {
		fmt.Fprintf(&b, "tools: %s\n", strings.Join(s.Tools, ", "))
	}
	if s.Model != "" {
		fmt.Fprintf(&b, "model: %s\n", s.Model)
	}
	b.WriteString("---\n\n")
	b.WriteString(withSingleTrailingNewline(s.Prompt))
	return b.String()
}

// RenderOpenCode renders a Source into OpenCode agent-file content.
//
// Mode is reserved for future subagent support: every generated OpenCode
// agent currently emits `mode: all` regardless of Source.Mode, because
// `opencode run --agent <name>` silently ignores `mode: subagent` at the
// top level.
func RenderOpenCode(s *Source) string {
	var b strings.Builder
	b.WriteString("---\n")
	fmt.Fprintf(&b, "description: %s\n", s.Description)
	b.WriteString("mode: all\n")
	if s.Model != "" {
		fmt.Fprintf(&b, "model: %s\n", s.Model)
	}
	if len(s.Tools) > 0 {
		mapped := make([]string, 0, len(s.Tools))
		for _, t := range s.Tools {
			if oc, ok := claudeToOpenCode[t]; ok {
				mapped = append(mapped, oc)
			}
		}
		if len(mapped) > 0 {
			sort.Strings(mapped)
			b.WriteString("tools:\n")
			for _, oc := range mapped {
				fmt.Fprintf(&b, "  %s: true\n", oc)
			}
		}
	}
	b.WriteString("---\n\n")
	b.WriteString(withSingleTrailingNewline(s.Prompt))
	return b.String()
}

// renderCodex is a reserved slot for the future Codex target. Codex is not
// implemented yet; ok is always false and no codex file is generated.
//
// TODO(codex): implement once the Codex agent-file format is settled.
func renderCodex(s *Source) (string, bool) {
	return "", false
}

// Targets returns every file that should be generated for a source.
func Targets(s *Source) []Target {
	targets := []Target{
		{Path: filepath.Join(".claude", "agents", s.Name+".md"), Content: RenderClaude(s)},
		{Path: filepath.Join(".opencode", "agents", s.Name+".md"), Content: RenderOpenCode(s)},
	}
	// TODO(codex): reserved slot — once renderCodex is implemented, append
	// its Target here (Codex is intentionally not generated today).
	if content, ok := renderCodex(s); ok {
		targets = append(targets, Target{Path: filepath.Join(".codex", "agents", s.Name+".md"), Content: content})
	}
	return targets
}

// Sync loads every source in agentsDir, renders all targets, and writes
// them under repoRoot. It returns the list of written repo-relative paths.
func Sync(agentsDir, repoRoot string) ([]string, error) {
	sources, err := LoadAll(agentsDir)
	if err != nil {
		return nil, err
	}
	var written []string
	for _, s := range sources {
		for _, t := range Targets(s) {
			full := filepath.Join(repoRoot, t.Path)
			if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
				return nil, fmt.Errorf("agentsync: mkdir for %s: %w", t.Path, err)
			}
			if err := os.WriteFile(full, []byte(t.Content), 0o644); err != nil {
				return nil, fmt.Errorf("agentsync: write %s: %w", t.Path, err)
			}
			written = append(written, t.Path)
		}
	}
	return written, nil
}

// Check loads every source in agentsDir, renders all targets, and compares
// them to what's on disk under repoRoot. It returns the repo-relative paths
// that are missing or different from their generated content. An empty
// slice means everything is in sync.
func Check(agentsDir, repoRoot string) ([]string, error) {
	sources, err := LoadAll(agentsDir)
	if err != nil {
		return nil, err
	}
	var drifted []string
	for _, s := range sources {
		for _, t := range Targets(s) {
			full := filepath.Join(repoRoot, t.Path)
			existing, err := os.ReadFile(full)
			if err != nil {
				drifted = append(drifted, t.Path)
				continue
			}
			if string(existing) != t.Content {
				drifted = append(drifted, t.Path)
			}
		}
	}
	return drifted, nil
}
