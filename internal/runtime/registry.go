package runtime

import (
	"fmt"
	"sort"
	"sync"
)

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

// namesLocked returns sorted registry names. Callers must hold registryMu.
func namesLocked() []string {
	names := make([]string, 0, len(registry))
	for name := range registry {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
