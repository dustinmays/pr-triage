package events_test

import (
	"fmt"
	"path/filepath"
	"sync"
	"testing"

	"github.com/dustinmays/pr-triage/internal/events"
)

func TestEmitter_MultipleSubscribers(t *testing.T) {
	emitter := events.NewEmitter()

	var mu sync.Mutex
	var received1 []events.Event
	var received2 []events.Event

	emitter.Subscribe(func(e events.Event) {
		mu.Lock()
		received1 = append(received1, e)
		mu.Unlock()
	})

	emitter.Subscribe(func(e events.Event) {
		mu.Lock()
		received2 = append(received2, e)
		mu.Unlock()
	})

	evt := events.Event{
		Type:      events.EventPRStateChanged,
		RepoOwner: "dustinmays",
		RepoName:  "pr-triage",
		PRNumber:  42,
		HeadSHA:   "sha-test-123",
		State:     "ci_running",
	}

	emitter.Emit(evt)

	mu.Lock()
	defer mu.Unlock()

	if len(received1) != 1 || received1[0].PRNumber != 42 {
		t.Fatalf("subscriber 1 failed to receive event: %v", received1)
	}
	if len(received2) != 1 || received2[0].PRNumber != 42 {
		t.Fatalf("subscriber 2 failed to receive event: %v", received2)
	}
}

func TestStatusFileWriter_EndToEnd(t *testing.T) {
	tmpDir := t.TempDir()
	statusFile := filepath.Join(tmpDir, "status.json")

	writer := events.NewStatusFileWriter(statusFile)
	emitter := events.NewEmitter()
	emitter.Subscribe(writer.HandleEvent)

	// 1. Emit PR state changed
	emitter.Emit(events.Event{
		Type:      events.EventPRStateChanged,
		RepoOwner: "dustinmays",
		RepoName:  "pr-triage",
		PRNumber:  10,
		HeadSHA:   "sha-10",
		State:     "report_ready",
	})

	status, err := events.ReadStatus(statusFile)
	if err != nil {
		t.Fatalf("ReadStatus failed: %v", err)
	}
	if len(status.RecentPRs) != 1 || status.RecentPRs[0].PRNumber != 10 || status.RecentPRs[0].State != "report_ready" {
		t.Errorf("unexpected recent PRs: %+v", status.RecentPRs)
	}
	if len(status.ActiveRuns) != 0 {
		t.Errorf("expected 0 active runs, got %d", len(status.ActiveRuns))
	}

	// 2. Emit Agent Started
	emitter.Emit(events.Event{
		Type:      events.EventAgentStarted,
		RepoOwner: "dustinmays",
		RepoName:  "pr-triage",
		PRNumber:  10,
		HeadSHA:   "sha-10",
		State:     "agent_running",
		Runtime:   "claude-code",
		Model:     "claude-3-7-sonnet",
	})

	status, err = events.ReadStatus(statusFile)
	if err != nil {
		t.Fatalf("ReadStatus after agent start failed: %v", err)
	}
	if len(status.ActiveRuns) != 1 || status.ActiveRuns[0].PRNumber != 10 || status.ActiveRuns[0].Runtime != "claude-code" {
		t.Errorf("unexpected active runs: %+v", status.ActiveRuns)
	}

	// 3. Emit Agent Finished
	emitter.Emit(events.Event{
		Type:       events.EventAgentFinished,
		RepoOwner:  "dustinmays",
		RepoName:   "pr-triage",
		PRNumber:   10,
		HeadSHA:    "sha-10",
		State:      "done",
		CostUSD:    0.15,
		CostBasis:  "exact",
		Turns:      4,
		StopReason: "completed",
	})

	status, err = events.ReadStatus(statusFile)
	if err != nil {
		t.Fatalf("ReadStatus after agent finish failed: %v", err)
	}
	if len(status.ActiveRuns) != 0 {
		t.Errorf("expected active runs cleared after finish, got %+v", status.ActiveRuns)
	}
	if status.RecentPRs[0].State != "done" {
		t.Errorf("expected PR state to be 'done', got %q", status.RecentPRs[0].State)
	}
}

func TestStatusFileWriter_RecentErrors_MixedSourcesTrackedTogether(t *testing.T) {
	tmpDir := t.TempDir()
	statusFile := filepath.Join(tmpDir, "status.json")

	writer := events.NewStatusFileWriter(statusFile)
	emitter := events.NewEmitter()
	emitter.Subscribe(writer.HandleEvent)

	emitter.Emit(events.Event{
		Type:        events.EventPollError,
		RepoOwner:   "Bellese",
		RepoName:    "orgz-seed-template",
		Description: "404 Not Found",
	})
	emitter.Emit(events.Event{
		Type:        events.EventOrchestratorError,
		RepoOwner:   "Bellese",
		RepoName:    "orgz-seed-template",
		PRNumber:    90,
		Description: "escalate: create comment: 403 Forbidden",
	})

	status, err := events.ReadStatus(statusFile)
	if err != nil {
		t.Fatalf("ReadStatus failed: %v", err)
	}
	if len(status.RecentErrors) != 2 {
		t.Fatalf("len(RecentErrors) = %d, want 2 (poll + orchestrator)", len(status.RecentErrors))
	}
	if status.RecentErrors[0].Type != events.EventPollError {
		t.Errorf("RecentErrors[0].Type = %q, want poll_error", status.RecentErrors[0].Type)
	}
	if status.RecentErrors[1].Type != events.EventOrchestratorError || status.RecentErrors[1].PRNumber != 90 {
		t.Errorf("RecentErrors[1] = %+v, want orchestrator_error scoped to PR 90", status.RecentErrors[1])
	}
}

func TestStatusFileWriter_PollErrors_TrackedAndCapped(t *testing.T) {
	tmpDir := t.TempDir()
	statusFile := filepath.Join(tmpDir, "status.json")

	writer := events.NewStatusFileWriter(statusFile)
	emitter := events.NewEmitter()
	emitter.Subscribe(writer.HandleEvent)

	// Emit more than the retention cap to verify old errors are trimmed.
	for i := 0; i < 15; i++ {
		emitter.Emit(events.Event{
			Type:        events.EventPollError,
			RepoOwner:   "Bellese",
			RepoName:    "orgz-seed-template",
			Description: fmt.Sprintf("404 Not Found (attempt %d)", i),
		})
	}

	status, err := events.ReadStatus(statusFile)
	if err != nil {
		t.Fatalf("ReadStatus failed: %v", err)
	}
	if len(status.RecentErrors) != 10 {
		t.Fatalf("len(RecentErrors) = %d, want 10 (capped)", len(status.RecentErrors))
	}
	last := status.RecentErrors[len(status.RecentErrors)-1]
	if last.Description != "404 Not Found (attempt 14)" {
		t.Errorf("most recent poll error = %q, want the last emitted one", last.Description)
	}
	if status.LastEvent == nil || status.LastEvent.Type != events.EventPollError {
		t.Errorf("LastEvent = %+v, want type poll_error", status.LastEvent)
	}
}
