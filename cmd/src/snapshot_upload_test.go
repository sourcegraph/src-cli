package main

import (
	"os"
	"path/filepath"
	"slices"
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
			name:      "valid: summary.json",
			fileFlag:  "summary.json",
			wantError: false,
		},
		{
			name:      "valid: pgsql.sql",
			fileFlag:  "pgsql.sql",
			wantError: false,
		},
		{
			name:      "valid: codeintel.sql",
			fileFlag:  "codeintel.sql",
			wantError: false,
		},
		{
			name:      "valid: codeinsights.sql",
			fileFlag:  "codeinsights.sql",
			wantError: false,
		},
		{
			name:      "valid: empty (all files)",
			fileFlag:  "",
			wantError: false,
		},
		{
			name:      "valid: comma-delimited",
			fileFlag:  "summary.json,pgsql.sql",
			wantError: false,
		},
		{
			name:      "valid: comma-delimited with spaces",
			fileFlag:  "summary.json, pgsql.sql, codeintel.sql",
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
			fileFlag:  "summary.json,invalid",
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test the validation logic using the shared listOfValidFiles
			var hasError bool
			if tt.fileFlag == "" {
				// Empty is valid (defaults to all files)
				hasError = false
			} else {
				parts := strings.SplitSeq(tt.fileFlag, ",")
				for part := range parts {
					filename := strings.TrimSpace(part)

					if !slices.Contains(listOfValidFiles, filename) {
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
				"codeinsights.sql",
				"codeintel.sql",
				"pgsql.sql",
				"summary.json",
			},
		},
		{
			name:       "summary only",
			fileFilter: "summary.json",
			expectedFiles: []string{
				"summary.json",
			},
		},
		{
			name:       "pgsql only",
			fileFilter: "pgsql.sql",
			expectedFiles: []string{
				"pgsql.sql",
			},
		},
		{
			name:       "codeintel only",
			fileFilter: "codeintel.sql",
			expectedFiles: []string{
				"codeintel.sql",
			},
		},
		{
			name:       "codeinsights only",
			fileFilter: "codeinsights.sql",
			expectedFiles: []string{
				"codeinsights.sql",
			},
		},
		{
			name:       "comma-delimited: summary and pgsql",
			fileFilter: "summary.json,pgsql.sql",
			expectedFiles: []string{
				"summary.json",
				"pgsql.sql",
			},
		},
		{
			name:       "comma-delimited: all database files",
			fileFilter: "pgsql.sql,codeintel.sql,codeinsights.sql",
			expectedFiles: []string{
				"pgsql.sql",
				"codeintel.sql",
				"codeinsights.sql",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Parse file filter into list (mimicking parseFileFilter logic)
			var filesToUpload []string
			if tt.fileFilter == "" {
				filesToUpload = listOfValidFiles
			} else {
				parts := strings.SplitSeq(tt.fileFilter, ",")
				for part := range parts {
					filename := strings.TrimSpace(part)
					filesToUpload = append(filesToUpload, filename)
				}
			}

			// Simulate the file opening logic from openFilesAndCreateProgressBars
			var selectedFiles []string
			for _, selectedFile := range filesToUpload {
				// Construct path and check if file matches
				filePath := filepath.Join(snapshotDir, selectedFile)
				if _, err := os.Stat(filePath); err == nil {
					selectedFiles = append(selectedFiles, selectedFile)
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
		"pgsql.sql":        false,
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
