package main

import (
	"testing"

	"github.com/dustinmays/pr-triage/internal/runtime"
)

// TestRuntimeAdaptersRegistered guards against the wiring bug where the
// claude-code adapter's self-registering init() never ran because nothing in the
// binary imported its package, leaving the registry empty and every run failing
// with "unknown runtime". This test lives in package main so it shares main.go's
// import graph; dropping the blank import there makes it fail.
func TestRuntimeAdaptersRegistered(t *testing.T) {
	if _, err := runtime.Get("claude-code"); err != nil {
		t.Fatalf("claude-code runtime not registered in the binary (registered: %v); "+
			"is the blank import of internal/runtime/claudecode present in main.go? err: %v",
			runtime.Names(), err)
	}
}
