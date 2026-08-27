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

// ReportCheckName is the name of the GitHub Check Run that carries the pre-scan
// report JSON in its output summary (published by .github/workflows/pr-prescan.yml).
// The daemon must ingest the report from THIS check run specifically — a commit
// has many check runs (lint, test, build, …) and only this one holds the report.
const ReportCheckName = "pr-prescan-report"

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
	Chunk         *ChunkInfo     `json:"chunk,omitempty"`
}

// PRInfo represents pull request metadata in the report.
type PRInfo struct {
	Number     int    `json:"number"`
	Title      string `json:"title"`
	Base       string `json:"base"`
	Head       string `json:"head"`
	TargetKind string `json:"target_kind,omitempty"`
	IssueRefs  []int  `json:"issue_refs,omitempty"`
}

// CIInfo represents CI execution status in the report.
type CIInfo struct {
	Status        string   `json:"status"`
	FailingChecks []string `json:"failing_checks,omitempty"`
}

// LargestFileInfo represents metrics for the largest changed file.
type LargestFileInfo struct {
	Path    string `json:"path"`
	Changed int    `json:"changed"`
}

// DiffInfo represents code change metrics in the report.
type DiffInfo struct {
	FilesChanged     int              `json:"files_changed,omitempty"`
	Insertions       int              `json:"insertions,omitempty"`
	Additions        int              `json:"additions,omitempty"`
	Deletions        int              `json:"deletions,omitempty"`
	SourceFiles      int              `json:"source_files,omitempty"`
	GeneratedFiles   int              `json:"generated_files,omitempty"`
	SourceInsertions int              `json:"source_insertions,omitempty"`
	TopLevelDirs     []string         `json:"top_level_dirs,omitempty"`
	LargestFile      *LargestFileInfo `json:"largest_file,omitempty"`
	Summary          string           `json:"summary,omitempty"`
}

// Evidence represents a piece of evidence supporting a signal detection.
type Evidence struct {
	File   string `json:"file,omitempty"`
	Line   *int   `json:"line,omitempty"`
	Detail string `json:"detail,omitempty"`
}

// UnmarshalJSON unmarshals either an evidence object or a legacy plain string.
func (e *Evidence) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		e.Detail = s
		return nil
	}
	type Alias Evidence
	var a Alias
	if err := json.Unmarshal(data, &a); err != nil {
		return err
	}
	*e = Evidence(a)
	return nil
}

// Signal represents a single detected architectural or risk signal.
type Signal struct {
	ID       string     `json:"id"`
	Present  bool       `json:"present"`
	Evidence []Evidence `json:"evidence,omitempty"`
}

// ChunkInfo represents metadata about pull requests merged into a chunk branch.
type ChunkInfo struct {
	Branch    string        `json:"branch,omitempty"`
	MergedPRs []ChunkPRInfo `json:"merged_prs,omitempty"`
}

// ChunkPRInfo represents an individual PR merged into a chunk branch.
type ChunkPRInfo struct {
	Number           int    `json:"number"`
	Title            string `json:"title,omitempty"`
	IssueRefs        []int  `json:"issue_refs,omitempty"`
	NeedsOwnerReview bool   `json:"needs_owner_review,omitempty"`
}

// CheckRunFetcher represents the interface to retrieve check run output.
type CheckRunFetcher interface {
	FetchCheckRunOutput(ctx context.Context, owner, repo string, checkRunID int64) (*gh.CheckRunOutput, error)
}

// RawReportSchemaJSON is the embedded v1 JSON schema.
const RawReportSchemaJSON = `{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "title": "CI PR Triage Report",
  "description": "Pre-scan JSON report generated in CI/CD pipeline that triggers agent triage",
  "type": "object",
  "required": [
    "schema_version",
    "pr",
    "ci",
    "stack",
    "diff",
    "signals"
  ],
  "additionalProperties": true,
  "properties": {
    "schema_version": {
      "type": "integer",
      "const": 1,
      "description": "Schema version number (must be 1 for v1 reports)"
    },
    "pr": {
      "type": "object",
      "required": [
        "number",
        "title",
        "base",
        "head"
      ],
      "additionalProperties": true,
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
            "type": "integer"
          }
        }
      }
    },
    "ci": {
      "type": "object",
      "required": [
        "status"
      ],
      "additionalProperties": true,
      "properties": {
        "status": {
          "type": "string",
          "enum": ["none", "failing", "pending", "passing"]
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
          "type": ["string", "null"]
        },
        "framework": {
          "type": ["string", "null"]
        },
        "orm": {
          "type": ["string", "null"]
        },
        "package_manager": {
          "type": ["string", "null"]
        },
        "linter": {
          "type": ["string", "null"]
        },
        "runtime": {
          "type": ["string", "null"]
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
        "insertions": {
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
        "source_files": {
          "type": "integer",
          "minimum": 0
        },
        "generated_files": {
          "type": "integer",
          "minimum": 0
        },
        "source_insertions": {
          "type": "integer",
          "minimum": 0
        },
        "top_level_dirs": {
          "type": "array",
          "items": {
            "type": "string"
          }
        },
        "largest_file": {
          "type": ["object", "null"],
          "properties": {
            "path": {
              "type": "string"
            },
            "changed": {
              "type": "integer",
              "minimum": 0
            }
          },
          "additionalProperties": true
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
        "additionalProperties": true,
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
              "type": ["object", "string"],
              "properties": {
                "file": {
                  "type": "string"
                },
                "line": {
                  "type": ["integer", "null"]
                },
                "detail": {
                  "type": "string"
                }
              },
              "additionalProperties": true
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
    },
    "chunk": {
      "type": ["object", "null"],
      "properties": {
        "branch": {
          "type": "string"
        },
        "merged_prs": {
          "type": "array",
          "items": {
            "type": "object",
            "required": [
              "number"
            ],
            "properties": {
              "number": {
                "type": "integer",
                "minimum": 1
              },
              "title": {
                "type": "string"
              },
              "issue_refs": {
                "type": "array",
                "items": {
                  "type": "integer"
                }
              },
              "needs_owner_review": {
                "type": "boolean"
              }
            },
            "additionalProperties": true
          }
        }
      },
      "additionalProperties": true
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
