package schema_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v5"
)

func TestReportSchema_Fixtures(t *testing.T) {
	schemaPath := filepath.Join("..", "schema", "report.schema.json")
	compiler := jsonschema.NewCompiler()
	compiler.Draft = jsonschema.Draft7

	schema, err := compiler.Compile(schemaPath)
	if err != nil {
		t.Fatalf("failed to compile schema %s: %v", schemaPath, err)
	}

	testCases := []struct {
		name      string
		file      string
		wantValid bool
	}{
		{
			name:      "valid v1 report",
			file:      filepath.Join("..", "testdata", "reports", "valid.json"),
			wantValid: true,
		},
		{
			name:      "high risk v1 report",
			file:      filepath.Join("..", "testdata", "reports", "high-risk.json"),
			wantValid: true,
		},
		{
			name:      "malformed report (missing required fields / wrong types)",
			file:      filepath.Join("..", "testdata", "reports", "malformed.json"),
			wantValid: false,
		},
		{
			name:      "unknown schema version",
			file:      filepath.Join("..", "testdata", "reports", "unknown-version.json"),
			wantValid: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			data, err := os.ReadFile(tc.file)
			if err != nil {
				t.Fatalf("failed to read fixture %s: %v", tc.file, err)
			}

			var val any
			if err := json.Unmarshal(data, &val); err != nil {
				t.Fatalf("failed to parse JSON in %s: %v", tc.file, err)
			}

			err = schema.Validate(val)
			if tc.wantValid && err != nil {
				t.Errorf("expected %s to be valid, got error: %v", tc.file, err)
			} else if !tc.wantValid && err == nil {
				t.Errorf("expected %s to be invalid, but validation succeeded", tc.file)
			}
		})
	}
}
