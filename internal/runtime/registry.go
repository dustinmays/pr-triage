package runtime

import (
	"fmt"
	"sort"
	"sync"
)

// Known runtime names.
const (
	NameClaudeCode = "claude-code"
	NameCodex      = "codex"
	NameOpenCode   = "opencode"
)

// DefaultName is the runtime selected when no explicit or configured runtime is provided.
const DefaultName = NameClaudeCode

// DefaultModel is the default model identifier for the default runtime.
const DefaultModel = "claude-3-7-sonnet"

var (
	registryMu sync.RWMutex
	registry   = map[string]AgentRuntime{}
)

// Register adds an AgentRuntime to the package registry under its Name().
// Adapters are expected to call Register from an init() function to
// self-register. Register panics if r is nil, r.Name() is empty, or a
// runtime is already registered under that name, since these are all
// programmer errors caught at startup, not runtime conditions.
func Register(r AgentRuntime) {
	if r == nil {
		panic("runtime: Register called with nil AgentRuntime")
	}
	name := r.Name()
	if name == "" {
		panic("runtime: Register called with empty Name()")
	}

	registryMu.Lock()
	defer registryMu.Unlock()

	if _, exists := registry[name]; exists {
		panic(fmt.Sprintf("runtime: runtime %q already registered", name))
	}
	registry[name] = r
}

// Get looks up a registered AgentRuntime by name. It returns a clean error
// if no runtime is registered under that name.
func Get(name string) (AgentRuntime, error) {
	registryMu.RLock()
	defer registryMu.RUnlock()

	r, ok := registry[name]
	if !ok {
		return nil, fmt.Errorf("runtime: unknown runtime %q (registered: %v)", name, namesLocked())
	}
	return r, nil
}

// Names returns the names of all currently registered runtimes, sorted.
func Names() []string {
	registryMu.RLock()
	defer registryMu.RUnlock()
	return namesLocked()
}

// KnownNames returns the names of all currently registered runtimes, sorted.
func KnownNames() []string {
	return Names()
}

// Validate returns nil if name refers to a registered runtime, and a clear
// error otherwise. An empty name is treated as a configuration bug, not as a
// request for the default — callers should resolve defaults before calling
// Validate.
func Validate(name string) error {
	if name == "" {
		return fmt.Errorf("runtime: name is empty (expected one of %v)", Names())
	}
	registryMu.RLock()
	_, ok := registry[name]
	registryMu.RUnlock()
	if !ok {
		return fmt.Errorf("runtime: unknown runtime %q (expected one of %v)", name, Names())
	}
	return nil
}

// namesLocked returns sorted registry names. Callers must hold registryMu.
func namesLocked() []string {
	names := make([]string, 0, len(registry))
	for name := range registry {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
