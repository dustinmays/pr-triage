// Package report retrieves, validates, and parses the pre-scan JSON report
// produced by CI/CD runs according to the v1 report schema contract.
package report

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	gh "github.com/google/go-github/v72/github"
	"github.com/santhosh-tekuri/jsonschema/v5"
)

// SupportedSchemaVersion is the currently supported report schema version.
const SupportedSchemaVersion = 1

// Typed report errors.
var (
	ErrMissing            = errors.New("report: missing report in check run output")
	ErrMalformed          = errors.New("report: malformed report")
	ErrUnsupportedVersion = errors.New("report: unsupported schema version")
)

// Report represents a validated, typed CI pre-scan triage report.
type Report struct {
	SchemaVersion int            `json:"schema_version"`
	PR            PRInfo         `json:"pr"`
	CI            CIInfo         `json:"ci"`
	Stack         map[string]any `json:"stack,omitempty"`
	Diff          DiffInfo       `json:"diff,omitempty"`
	Signals       []Signal       `json:"signals"`
	Notes         []string       `json:"notes,omitempty"`
}

// PRInfo represents pull request metadata in the report.
type PRInfo struct {
	Number     int      `json:"number"`
	Title      string   `json:"title"`
	Base       string   `json:"base"`
	Head       string   `json:"head"`
	TargetKind string   `json:"target_kind,omitempty"`
	IssueRefs  []string `json:"issue_refs,omitempty"`
}

// CIInfo represents CI execution status in the report.
type CIInfo struct {
	Status        string   `json:"status"`
	FailingChecks []string `json:"failing_checks,omitempty"`
}

// DiffInfo represents code change metrics in the report.
type DiffInfo struct {
	FilesChanged int    `json:"files_changed,omitempty"`
	Additions    int    `json:"additions,omitempty"`
	Deletions    int    `json:"deletions,omitempty"`
	Summary      string `json:"summary,omitempty"`
}

// Signal represents a single detected architectural or risk signal.
type Signal struct {
	ID       string   `json:"id"`
	Present  bool     `json:"present"`
	Evidence []string `json:"evidence,omitempty"`
}

// CheckRunFetcher represents the interface to retrieve check run output.
type CheckRunFetcher interface {
	FetchCheckRunOutput(ctx context.Context, owner, repo string, checkRunID int64) (*gh.CheckRunOutput, error)
}

// RawReportSchemaJSON is the embedded v1 JSON schema.
const RawReportSchemaJSON = `{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "title": "CI PR Triage Report",
  "type": "object",
  "required": [
    "schema_version",
    "pr",
    "ci",
    "stack",
    "diff",
    "signals"
  ],
  "additionalProperties": false,
  "properties": {
    "schema_version": {
      "type": "integer",
      "const": 1
    },
    "pr": {
      "type": "object",
      "required": [
        "number",
        "title",
        "base",
        "head"
      ],
      "additionalProperties": false,
      "properties": {
        "number": {
          "type": "integer",
          "minimum": 1
        },
        "title": {
          "type": "string"
        },
        "base": {
          "type": "string"
        },
        "head": {
          "type": "string"
        },
        "target_kind": {
          "type": "string"
        },
        "issue_refs": {
          "type": "array",
          "items": {
            "type": "string"
          }
        }
      }
    },
    "ci": {
      "type": "object",
      "required": [
        "status"
      ],
      "additionalProperties": false,
      "properties": {
        "status": {
          "type": "string",
          "enum": ["passed", "failed", "running", "skipped"]
        },
        "failing_checks": {
          "type": "array",
          "items": {
            "type": "string"
          }
        }
      }
    },
    "stack": {
      "type": "object",
      "properties": {
        "language": {
          "type": "string"
        },
        "framework": {
          "type": "string"
        },
        "runtime": {
          "type": "string"
        }
      },
      "additionalProperties": true
    },
    "diff": {
      "type": "object",
      "properties": {
        "files_changed": {
          "type": "integer",
          "minimum": 0
        },
        "additions": {
          "type": "integer",
          "minimum": 0
        },
        "deletions": {
          "type": "integer",
          "minimum": 0
        },
        "summary": {
          "type": "string"
        }
      },
      "additionalProperties": true
    },
    "signals": {
      "type": "array",
      "items": {
        "type": "object",
        "required": [
          "id",
          "present"
        ],
        "additionalProperties": false,
        "properties": {
          "id": {
            "type": "string",
            "minLength": 1
          },
          "present": {
            "type": "boolean"
          },
          "evidence": {
            "type": "array",
            "items": {
              "type": "string"
            }
          }
        }
      }
    },
    "notes": {
      "type": "array",
      "items": {
        "type": "string"
      }
    }
  }
}`

var (
	compiledSchema     *jsonschema.Schema
	compiledSchemaOnce sync.Once
	compiledSchemaErr  error
)

func getSchema() (*jsonschema.Schema, error) {
	compiledSchemaOnce.Do(func() {
		compiler := jsonschema.NewCompiler()
		compiler.Draft = jsonschema.Draft7
		if err := compiler.AddResource("report.schema.json", strings.NewReader(RawReportSchemaJSON)); err != nil {
			compiledSchemaErr = fmt.Errorf("add schema resource: %w", err)
			return
		}
		schema, err := compiler.Compile("report.schema.json")
		if err != nil {
			compiledSchemaErr = fmt.Errorf("compile schema: %w", err)
			return
		}
		compiledSchema = schema
	})
	return compiledSchema, compiledSchemaErr
}

// FetchAndValidate retrieves check run output from GitHub and validates it against the v1 schema.
func FetchAndValidate(ctx context.Context, fetcher CheckRunFetcher, owner, repo string, checkRunID int64) (*Report, error) {
	if fetcher == nil {
		return nil, fmt.Errorf("report: fetcher cannot be nil")
	}

	output, err := fetcher.FetchCheckRunOutput(ctx, owner, repo, checkRunID)
	if err != nil {
		return nil, fmt.Errorf("report: fetch check run output %s/%s#%d: %w", owner, repo, checkRunID, err)
	}

	if output == nil {
		return nil, ErrMissing
	}

	raw := extractReportPayload(output)
	if strings.TrimSpace(raw) == "" {
		return nil, ErrMissing
	}

	return ParseAndValidate([]byte(raw))
}

// extractReportPayload checks summary and text fields for the JSON report payload.
func extractReportPayload(output *gh.CheckRunOutput) string {
	if output == nil {
		return ""
	}
	summary := strings.TrimSpace(output.GetSummary())
	if summary != "" {
		return summary
	}
	text := strings.TrimSpace(output.GetText())
	if text != "" {
		return text
	}
	return ""
}

// ParseAndValidate parses raw JSON bytes, validates against the v1 schema, and returns a typed Report.
func ParseAndValidate(data []byte) (*Report, error) {
	if len(strings.TrimSpace(string(data))) == 0 {
		return nil, ErrMissing
	}

	var rawMap map[string]any
	if err := json.Unmarshal(data, &rawMap); err != nil {
		return nil, fmt.Errorf("%w: invalid json: %v", ErrMalformed, err)
	}

	// Probe schema_version field
	if verVal, ok := rawMap["schema_version"]; ok {
		switch v := verVal.(type) {
		case float64:
			if int(v) != SupportedSchemaVersion {
				return nil, fmt.Errorf("%w: got version %d, want %d", ErrUnsupportedVersion, int(v), SupportedSchemaVersion)
			}
		default:
			return nil, fmt.Errorf("%w: schema_version is not an integer", ErrMalformed)
		}
	} else {
		return nil, fmt.Errorf("%w: missing schema_version", ErrMalformed)
	}

	schema, err := getSchema()
	if err != nil {
		return nil, fmt.Errorf("report schema error: %w", err)
	}

	if err := schema.Validate(rawMap); err != nil {
		return nil, fmt.Errorf("%w: schema validation failed: %v", ErrMalformed, err)
	}

	var rep Report
	if err := json.Unmarshal(data, &rep); err != nil {
		return nil, fmt.Errorf("%w: unmarshal into report struct: %v", ErrMalformed, err)
	}

	return &rep, nil
}
