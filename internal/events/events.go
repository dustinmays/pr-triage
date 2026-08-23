// Package events provides an internal publish-subscribe event emitter
// and status file writer for observability across the daemon, CLI, and SwiftBar.
package events

import (
	"sync"
	"time"
)

// EventType represents categorized daemon lifecycle and state events.
type EventType string

const (
	EventPollSweep      EventType = "poll_sweep"
	EventPRStateChanged EventType = "pr_state_changed"
	EventReportReady    EventType = "report_ready"
	EventAgentStarted   EventType = "agent_started"
	EventAgentFinished  EventType = "agent_finished"
	EventEscalated      EventType = "escalated"
)

// Event describes a single observability event.
type Event struct {
	Type        EventType `json:"type"`
	Timestamp   time.Time `json:"timestamp"`
	RepoOwner   string    `json:"repo_owner,omitempty"`
	RepoName    string    `json:"repo_name,omitempty"`
	PRNumber    int       `json:"pr_number,omitempty"`
	HeadSHA     string    `json:"head_sha,omitempty"`
	State       string    `json:"state,omitempty"`
	RunID       *int64    `json:"run_id,omitempty"`
	RiskTier    string    `json:"risk_tier,omitempty"`
	Runtime     string    `json:"runtime,omitempty"`
	Model       string    `json:"model,omitempty"`
	CostUSD     float64   `json:"cost_usd,omitempty"`
	CostBasis   string    `json:"cost_basis,omitempty"`
	Turns       int       `json:"turns,omitempty"`
	StopReason  string    `json:"stop_reason,omitempty"`
	Description string    `json:"description,omitempty"`
}

// Subscriber is a callback function that receives emitted events.
type Subscriber func(event Event)

// Emitter manages event subscriptions and dispatches events to all subscribers.
type Emitter struct {
	mu          sync.RWMutex
	subscribers []Subscriber
}

// NewEmitter creates a new Emitter instance.
func NewEmitter() *Emitter {
	return &Emitter{
		subscribers: make([]Subscriber, 0),
	}
}

// Subscribe adds a new subscriber callback.
func (e *Emitter) Subscribe(sub Subscriber) {
	if sub == nil {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.subscribers = append(e.subscribers, sub)
}

// Emit broadcasts an event to all registered subscribers.
func (e *Emitter) Emit(event Event) {
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now().UTC()
	}

	e.mu.RLock()
	subs := make([]Subscriber, len(e.subscribers))
	copy(subs, e.subscribers)
	e.mu.RUnlock()

	for _, sub := range subs {
		sub(event)
	}
}
