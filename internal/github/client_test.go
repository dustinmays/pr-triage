package github

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestClient_ListOpenPRs(t *testing.T) {
	prs := []map[string]any{
		{
			"number": 10,
			"title":  "PR 10",
			"state":  "open",
			"base": map[string]any{
				"ref": "main",
			},
			"head": map[string]any{
				"sha": "sha-pr-10",
			},
		},
		{
			"number": 11,
			"title":  "PR 11",
			"state":  "open",
			"base": map[string]any{
				"ref": "release/v1.0",
			},
			"head": map[string]any{
				"sha": "sha-pr-11",
			},
		},
		{
			"number": 12,
			"title":  "PR 12",
			"state":  "open",
			"base": map[string]any{
				"ref": "feature/test",
			},
			"head": map[string]any{
				"sha": "sha-pr-12",
			},
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/repos/owner/repo/pulls" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(prs)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	client := NewClientWithBaseURL("test-token", srv.URL)
	ctx := context.Background()

	// 1. All open PRs
	all, err := client.ListOpenPRs(ctx, "owner", "repo", "")
	if err != nil {
		t.Fatalf("ListOpenPRs failed: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("expected 3 PRs, got %d", len(all))
	}

	// 2. Exact match
	mainPRs, err := client.ListOpenPRs(ctx, "owner", "repo", "main")
	if err != nil {
		t.Fatalf("ListOpenPRs failed: %v", err)
	}
	if len(mainPRs) != 1 || mainPRs[0].GetNumber() != 10 {
		t.Fatalf("expected PR 10, got %v", mainPRs)
	}

	// 3. Glob match
	releasePRs, err := client.ListOpenPRs(ctx, "owner", "repo", "release/*")
	if err != nil {
		t.Fatalf("ListOpenPRs failed: %v", err)
	}
	if len(releasePRs) != 1 || releasePRs[0].GetNumber() != 11 {
		t.Fatalf("expected PR 11, got %v", releasePRs)
	}
}

func TestClient_GetPRHeadSHA(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/repos/owner/repo/pulls/42" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"number": 42,
				"title":  "Test PR",
				"head": map[string]any{
					"sha": "abcdef1234567890",
				},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	client := NewClientWithBaseURL("test-token", srv.URL)
	ctx := context.Background()

	sha, err := client.GetPRHeadSHA(ctx, "owner", "repo", 42)
	if err != nil {
		t.Fatalf("GetPRHeadSHA failed: %v", err)
	}
	if sha != "abcdef1234567890" {
		t.Fatalf("expected abcdef1234567890, got %s", sha)
	}
}

func TestClient_CheckRunsAndOutput(t *testing.T) {
	checkRunsResp := map[string]any{
		"total_count": 1,
		"check_runs": []map[string]any{
			{
				"id":         999,
				"name":       "ci/report",
				"head_sha":   "sha-xyz",
				"status":     "completed",
				"conclusion": "success",
				"output": map[string]any{
					"title":   "CI Report",
					"summary": `{"schema_version":1,"status":"passed"}`,
					"text":    "details here",
				},
			},
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/owner/repo/commits/sha-xyz/check-runs":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(checkRunsResp)
		case "/repos/owner/repo/check-runs/999":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(checkRunsResp["check_runs"].([]map[string]any)[0])
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	client := NewClientWithBaseURL("test-token", srv.URL)
	ctx := context.Background()

	runs, err := client.ListCheckRunsForSHA(ctx, "owner", "repo", "sha-xyz")
	if err != nil {
		t.Fatalf("ListCheckRunsForSHA failed: %v", err)
	}
	if len(runs) != 1 || runs[0].GetID() != 999 {
		t.Fatalf("expected check run 999, got %v", runs)
	}

	out, err := client.FetchCheckRunOutput(ctx, "owner", "repo", 999)
	if err != nil {
		t.Fatalf("FetchCheckRunOutput failed: %v", err)
	}
	if out == nil || out.GetTitle() != "CI Report" || out.GetSummary() != `{"schema_version":1,"status":"passed"}` {
		t.Fatalf("unexpected check run output: %+v", out)
	}
}

func TestClient_ETagAnd304Replay(t *testing.T) {
	var requestCount atomic.Int32
	var receivedIfNoneMatch string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := requestCount.Add(1)
		receivedIfNoneMatch = r.Header.Get("If-None-Match")

		if count > 1 && receivedIfNoneMatch == `"etag-test-123"` {
			w.WriteHeader(http.StatusNotModified)
			return
		}

		w.Header().Set("ETag", `"etag-test-123"`)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"number": 1,
			"title":  "Cached PR",
			"head": map[string]any{
				"sha": "cached-sha",
			},
		})
	}))
	defer srv.Close()

	client := NewClientWithBaseURL("test-token", srv.URL)
	ctx := context.Background()

	// 1st request -> cached
	sha1, err := client.GetPRHeadSHA(ctx, "owner", "repo", 1)
	if err != nil {
		t.Fatalf("1st request failed: %v", err)
	}
	if sha1 != "cached-sha" {
		t.Fatalf("expected cached-sha, got %s", sha1)
	}
	if requestCount.Load() != 1 {
		t.Fatalf("expected 1 request, got %d", requestCount.Load())
	}

	// 2nd request -> 304 replayed
	sha2, err := client.GetPRHeadSHA(ctx, "owner", "repo", 1)
	if err != nil {
		t.Fatalf("2nd request failed: %v", err)
	}
	if sha2 != "cached-sha" {
		t.Fatalf("expected cached-sha on 304 replay, got %s", sha2)
	}
	if requestCount.Load() != 2 {
		t.Fatalf("expected 2 requests, got %d", requestCount.Load())
	}
	if receivedIfNoneMatch != `"etag-test-123"` {
		t.Fatalf("expected If-None-Match header %q, got %q", `"etag-test-123"`, receivedIfNoneMatch)
	}
}

func TestClient_AddLabelsAndComment(t *testing.T) {
	var labelCalled, commentCalled bool

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/owner/repo/issues/1/labels":
			labelCalled = true
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{"name": "escalated"},
			})
		case "/repos/owner/repo/issues/1/comments":
			commentCalled = true
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":   555,
				"body": "escalating to @dustin",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	client := NewClientWithBaseURL("test-token", srv.URL)
	ctx := context.Background()

	err := client.AddLabels(ctx, "owner", "repo", 1, []string{"escalated"})
	if err != nil {
		t.Fatalf("AddLabels failed: %v", err)
	}
	if !labelCalled {
		t.Fatalf("AddLabels was not called on server")
	}

	cid, err := client.CreateComment(ctx, "owner", "repo", 1, "escalating to @dustin")
	if err != nil {
		t.Fatalf("CreateComment failed: %v", err)
	}
	if !commentCalled || cid != 555 {
		t.Fatalf("expected comment ID 555, got %d", cid)
	}
}
