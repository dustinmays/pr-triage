package report_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	gh "github.com/google/go-github/v72/github"

	"github.com/dustinmays/pr-triage/internal/github"
	"github.com/dustinmays/pr-triage/internal/report"
)

func TestParseAndValidate_ValidReport(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "reports", "valid.json"))
	if err != nil {
		t.Fatalf("failed to read valid.json: %v", err)
	}

	rep, err := report.ParseAndValidate(data)
	if err != nil {
		t.Fatalf("expected valid report, got error: %v", err)
	}

	if rep.SchemaVersion != 1 {
		t.Errorf("rep.SchemaVersion = %d, want 1", rep.SchemaVersion)
	}
	if rep.PR.Number != 42 {
		t.Errorf("rep.PR.Number = %d, want 42", rep.PR.Number)
	}
	if rep.PR.Title != "feat: add user profile picture upload endpoint" {
		t.Errorf("unexpected PR title: %s", rep.PR.Title)
	}
	if rep.CI.Status != "passed" {
		t.Errorf("rep.CI.Status = %s, want passed", rep.CI.Status)
	}
	if len(rep.Signals) != 5 {
		t.Errorf("len(rep.Signals) = %d, want 5", len(rep.Signals))
	}
	for _, sig := range rep.Signals {
		if sig.Present {
			t.Errorf("expected signal %s to be present:false in valid.json", sig.ID)
		}
	}
}

func TestParseAndValidate_HighRiskReport(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "reports", "high-risk.json"))
	if err != nil {
		t.Fatalf("failed to read high-risk.json: %v", err)
	}

	rep, err := report.ParseAndValidate(data)
	if err != nil {
		t.Fatalf("expected high-risk report to validate, got error: %v", err)
	}

	if rep.PR.Number != 43 {
		t.Errorf("rep.PR.Number = %d, want 43", rep.PR.Number)
	}

	var foundSchemaSignal bool
	for _, sig := range rep.Signals {
		if sig.ID == "schema_changed_without_migration" && sig.Present {
			foundSchemaSignal = true
			if len(sig.Evidence) == 0 {
				t.Errorf("expected evidence for schema_changed_without_migration")
			}
		}
	}
	if !foundSchemaSignal {
		t.Errorf("expected schema_changed_without_migration present:true in high-risk.json")
	}
}

func TestParseAndValidate_MalformedReport(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "reports", "malformed.json"))
	if err != nil {
		t.Fatalf("failed to read malformed.json: %v", err)
	}

	_, err = report.ParseAndValidate(data)
	if err == nil {
		t.Fatalf("expected malformed report to fail validation, got nil error")
	}
	if !errors.Is(err, report.ErrMalformed) {
		t.Errorf("expected ErrMalformed, got %v", err)
	}
}

func TestParseAndValidate_UnknownVersion(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "reports", "unknown-version.json"))
	if err != nil {
		t.Fatalf("failed to read unknown-version.json: %v", err)
	}

	_, err = report.ParseAndValidate(data)
	if err == nil {
		t.Fatalf("expected unknown version report to fail validation, got nil error")
	}
	if !errors.Is(err, report.ErrUnsupportedVersion) {
		t.Errorf("expected ErrUnsupportedVersion, got %v", err)
	}
}

func TestParseAndValidate_Empty(t *testing.T) {
	_, err := report.ParseAndValidate([]byte(""))
	if !errors.Is(err, report.ErrMissing) {
		t.Errorf("expected ErrMissing for empty data, got %v", err)
	}

	_, err = report.ParseAndValidate([]byte("   "))
	if !errors.Is(err, report.ErrMissing) {
		t.Errorf("expected ErrMissing for whitespace data, got %v", err)
	}
}

func TestFetchAndValidate_HTTPMock(t *testing.T) {
	validData, _ := os.ReadFile(filepath.Join("..", "..", "testdata", "reports", "valid.json"))
	malformedData, _ := os.ReadFile(filepath.Join("..", "..", "testdata", "reports", "malformed.json"))
	unknownVerData, _ := os.ReadFile(filepath.Join("..", "..", "testdata", "reports", "unknown-version.json"))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/repos/owner/repo/check-runs/100":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": 100,
				"output": map[string]any{
					"title":   "Valid Report",
					"summary": string(validData),
				},
			})
		case "/repos/owner/repo/check-runs/101":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": 101,
				"output": map[string]any{
					"title": "Malformed Report",
					"text":  string(malformedData),
				},
			})
		case "/repos/owner/repo/check-runs/102":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": 102,
				"output": map[string]any{
					"title":   "Unknown Version Report",
					"summary": string(unknownVerData),
				},
			})
		case "/repos/owner/repo/check-runs/103":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":     103,
				"output": map[string]any{},
			})
		case "/repos/owner/repo/check-runs/104":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": 104,
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	client := github.NewClientWithBaseURL("test-token", srv.URL)
	ctx := context.Background()

	// 100: Valid report
	rep, err := report.FetchAndValidate(ctx, client, "owner", "repo", 100)
	if err != nil {
		t.Fatalf("fetch 100 failed: %v", err)
	}
	if rep.PR.Number != 42 {
		t.Errorf("rep.PR.Number = %d, want 42", rep.PR.Number)
	}

	// 101: Malformed report
	_, err = report.FetchAndValidate(ctx, client, "owner", "repo", 101)
	if !errors.Is(err, report.ErrMalformed) {
		t.Errorf("expected ErrMalformed for check run 101, got %v", err)
	}

	// 102: Unknown version report
	_, err = report.FetchAndValidate(ctx, client, "owner", "repo", 102)
	if !errors.Is(err, report.ErrUnsupportedVersion) {
		t.Errorf("expected ErrUnsupportedVersion for check run 102, got %v", err)
	}

	// 103: Empty output report
	_, err = report.FetchAndValidate(ctx, client, "owner", "repo", 103)
	if !errors.Is(err, report.ErrMissing) {
		t.Errorf("expected ErrMissing for check run 103, got %v", err)
	}

	// 104: Nil output report
	_, err = report.FetchAndValidate(ctx, client, "owner", "repo", 104)
	if !errors.Is(err, report.ErrMissing) {
		t.Errorf("expected ErrMissing for check run 104, got %v", err)
	}
}

type mockFetcher struct {
	output *gh.CheckRunOutput
	err    error
}

func (m *mockFetcher) FetchCheckRunOutput(ctx context.Context, owner, repo string, checkRunID int64) (*gh.CheckRunOutput, error) {
	return m.output, m.err
}

func TestFetchAndValidate_NilFetcher(t *testing.T) {
	_, err := report.FetchAndValidate(context.Background(), nil, "owner", "repo", 1)
	if err == nil {
		t.Fatal("expected error with nil fetcher")
	}
}

func TestFetchAndValidate_FetcherError(t *testing.T) {
	mf := &mockFetcher{err: errors.New("network down")}
	_, err := report.FetchAndValidate(context.Background(), mf, "owner", "repo", 1)
	if err == nil || !errors.Is(err, mf.err) && err.Error() == "" {
		t.Fatalf("expected network error, got %v", err)
	}
}
