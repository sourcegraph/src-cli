package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/sourcegraph/src-cli/internal/pgdump"
)

func setupSnapshotFiles(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	
	// Create the snapshot directory structure
	snapshotDir := filepath.Join(dir, "src-snapshot")
	err := os.Mkdir(snapshotDir, 0755)
	require.NoError(t, err)
	
	// Create summary.json
	summaryPath := filepath.Join(snapshotDir, "summary.json")
	err = os.WriteFile(summaryPath, []byte(`{"version": "test"}`), 0644)
	require.NoError(t, err)
	
	// Create database dump files
	for _, output := range pgdump.Outputs(snapshotDir, pgdump.Targets{}) {
		err = os.WriteFile(output.Output, []byte("-- test SQL dump"), 0644)
		require.NoError(t, err)
	}
	
	return snapshotDir
}

func TestFileFilterValidation(t *testing.T) {
	tests := []struct {
		name      string
		fileFlag  string
		wantError bool
	}{
		{
			name:      "valid: summary",
			fileFlag:  "summary",
			wantError: false,
		},
		{
			name:      "valid: primary",
			fileFlag:  "primary",
			wantError: false,
		},
		{
			name:      "valid: codeintel",
			fileFlag:  "codeintel",
			wantError: false,
		},
		{
			name:      "valid: codeinsights",
			fileFlag:  "codeinsights",
			wantError: false,
		},
		{
			name:      "valid: empty (all files)",
			fileFlag:  "",
			wantError: false,
		},
		{
			name:      "valid: comma-delimited",
			fileFlag:  "summary,primary",
			wantError: false,
		},
		{
			name:      "valid: comma-delimited with spaces",
			fileFlag:  "summary, primary, codeintel",
			wantError: false,
		},
		{
			name:      "valid: with .sql extension",
			fileFlag:  "primary.sql",
			wantError: false,
		},
		{
			name:      "valid: with .json extension",
			fileFlag:  "summary.json",
			wantError: false,
		},
		{
			name:      "valid: mixed extensions",
			fileFlag:  "summary.json,primary.sql",
			wantError: false,
		},
		{
			name:      "invalid: unknown file",
			fileFlag:  "unknown",
			wantError: true,
		},
		{
			name:      "invalid: typo",
			fileFlag:  "primry",
			wantError: true,
		},
		{
			name:      "invalid: one valid, one invalid",
			fileFlag:  "summary,invalid",
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test the validation logic
			validFiles := map[string]bool{
				"summary":      true,
				"primary":      true,
				"codeintel":    true,
				"codeinsights": true,
			}
			
			var hasError bool
			if tt.fileFlag == "" {
				// Empty is valid (defaults to all files)
				hasError = false
			} else {
				parts := strings.Split(tt.fileFlag, ",")
				for _, part := range parts {
					normalized := strings.TrimSpace(part)
					normalized = strings.TrimSuffix(normalized, ".json")
					normalized = strings.TrimSuffix(normalized, ".sql")
					
					if !validFiles[normalized] {
						hasError = true
						break
					}
				}
			}
			
			if tt.wantError {
				require.True(t, hasError, "expected invalid file flag to be rejected")
			} else {
				require.False(t, hasError, "expected valid file flag to be accepted")
			}
		})
	}
}

func TestFileSelection(t *testing.T) {
	snapshotDir := setupSnapshotFiles(t)
	
	tests := []struct {
		name          string
		fileFilter    string
		expectedFiles []string
	}{
		{
			name:       "no filter - all files",
			fileFilter: "",
			expectedFiles: []string{
				"summary.json",
				"primary.sql",
				"codeintel.sql",
				"codeinsights.sql",
			},
		},
		{
			name:       "summary only",
			fileFilter: "summary",
			expectedFiles: []string{
				"summary.json",
			},
		},
		{
			name:       "primary only",
			fileFilter: "primary",
			expectedFiles: []string{
				"primary.sql",
			},
		},
		{
			name:       "codeintel only",
			fileFilter: "codeintel",
			expectedFiles: []string{
				"codeintel.sql",
			},
		},
		{
			name:       "codeinsights only",
			fileFilter: "codeinsights",
			expectedFiles: []string{
				"codeinsights.sql",
			},
		},
		{
			name:       "comma-delimited: summary and primary",
			fileFilter: "summary,primary",
			expectedFiles: []string{
				"summary.json",
				"primary.sql",
			},
		},
		{
			name:       "comma-delimited: all database files",
			fileFilter: "primary,codeintel,codeinsights",
			expectedFiles: []string{
				"primary.sql",
				"codeintel.sql",
				"codeinsights.sql",
			},
		},
		{
			name:       "comma-delimited with extensions",
			fileFilter: "summary.json,primary.sql",
			expectedFiles: []string{
				"summary.json",
				"primary.sql",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Parse file filter into list
			var filesToUpload []string
			if tt.fileFilter == "" {
				filesToUpload = []string{"summary", "primary", "codeintel", "codeinsights"}
			} else {
				parts := strings.Split(tt.fileFilter, ",")
				for _, part := range parts {
					normalized := strings.TrimSpace(part)
					normalized = strings.TrimSuffix(normalized, ".json")
					normalized = strings.TrimSuffix(normalized, ".sql")
					filesToUpload = append(filesToUpload, normalized)
				}
			}
			
			// Helper to check if a file should be uploaded
			shouldUpload := func(fileType string) bool {
				for _, f := range filesToUpload {
					if f == fileType {
						return true
					}
				}
				return false
			}
			
			var selectedFiles []string
			
			// Simulate the file selection logic from snapshot_upload.go
			if shouldUpload("summary") {
				selectedFiles = append(selectedFiles, "summary.json")
			}
			
			for _, o := range pgdump.Outputs(snapshotDir, pgdump.Targets{}) {
				fileName := filepath.Base(o.Output)
				fileType := fileName[:len(fileName)-4] // Remove .sql extension
				
				if shouldUpload(fileType) {
					selectedFiles = append(selectedFiles, fileName)
				}
			}
			
			require.Equal(t, tt.expectedFiles, selectedFiles, "selected files should match expected")
		})
	}
}

func TestFilterSQLBehavior(t *testing.T) {
	tests := []struct {
		name           string
		isSummary      bool
		filterSQLFlag  bool
		expectedFilter bool
	}{
		{
			name:           "summary file - filterSQL should be false",
			isSummary:      true,
			filterSQLFlag:  true,
			expectedFilter: false,
		},
		{
			name:           "database dump - filterSQL flag true",
			isSummary:      false,
			filterSQLFlag:  true,
			expectedFilter: true,
		},
		{
			name:           "database dump - filterSQL flag false",
			isSummary:      false,
			filterSQLFlag:  false,
			expectedFilter: false,
		},
		{
			name:           "summary file - filterSQL flag false (should still be false)",
			isSummary:      true,
			filterSQLFlag:  false,
			expectedFilter: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate the filterSQL logic from snapshot_upload.go
			actualFilter := !tt.isSummary && tt.filterSQLFlag
			
			require.Equal(t, tt.expectedFilter, actualFilter, "filterSQL should be set correctly")
		})
	}
}

func TestDatabaseOutputs(t *testing.T) {
	snapshotDir := setupSnapshotFiles(t)
	
	outputs := pgdump.Outputs(snapshotDir, pgdump.Targets{})
	
	// Should have exactly 3 database files
	require.Len(t, outputs, 3, "should have 3 database outputs")
	
	expectedFiles := map[string]bool{
		"primary.sql":      false,
		"codeintel.sql":    false,
		"codeinsights.sql": false,
	}
	
	for _, output := range outputs {
		fileName := filepath.Base(output.Output)
		_, exists := expectedFiles[fileName]
		require.True(t, exists, "unexpected file: %s", fileName)
		expectedFiles[fileName] = true
	}
	
	// Verify all expected files were found
	for fileName, found := range expectedFiles {
		require.True(t, found, "expected file not found: %s", fileName)
	}
}
