package blueprint

import (
	"testing"
)

func TestResolveBlueprintSource(t *testing.T) {
	tests := []struct {
		name     string
		repo     string
		rev      string
		subdir   string
		wantErr  string
		wantType string
	}{
		{
			name:     "valid https url",
			repo:     "https://github.com/org/blueprints",
			wantType: "git",
		},
		{
			name:     "valid https url with rev",
			repo:     "https://github.com/org/blueprints",
			rev:      "v1.0.0",
			wantType: "git",
		},
		{
			name:     "valid https url with subdir",
			repo:     "https://github.com/org/blueprints",
			subdir:   "monitors/cve-2025-1234",
			wantType: "git",
		},
		{
			name:    "ssh url rejected",
			repo:    "git@github.com:org/blueprints.git",
			wantErr: "invalid repository URL",
		},
		{
			name:    "http url rejected",
			repo:    "http://github.com/org/blueprints",
			wantErr: "unsupported URL scheme",
		},
		{
			name:     "absolute local path",
			repo:     "/path/to/local/repo",
			wantType: "local",
		},
		{
			name:     "relative path with dot slash",
			repo:     "./local/repo",
			wantType: "local",
		},
		{
			name:     "relative path with parent",
			repo:     "../other/repo",
			wantType: "local",
		},
		{
			name:     "current directory",
			repo:     ".",
			wantType: "local",
		},
		{
			name:     "parent directory",
			repo:     "..",
			wantType: "local",
		},
		{
			name:    "absolute subdir rejected",
			repo:    "https://github.com/org/blueprints",
			subdir:  "/etc/passwd",
			wantErr: "subdir must be a relative path",
		},
		{
			name:    "parent escape subdir rejected",
			repo:    "https://github.com/org/blueprints",
			subdir:  "../../../etc/passwd",
			wantErr: "subdir must not escape the repository root",
		},
		{
			name:    "hidden parent escape rejected",
			repo:    "https://github.com/org/blueprints",
			subdir:  "foo/../../bar",
			wantErr: "subdir must not escape the repository root",
		},
		{
			name:    "local path with absolute subdir rejected",
			repo:    "/path/to/repo",
			subdir:  "/etc/passwd",
			wantErr: "subdir must be a relative path",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src, err := ResolveBlueprintSource(tt.repo, tt.rev, tt.subdir)

			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if !contains(err.Error(), tt.wantErr) {
					t.Fatalf("expected error containing %q, got %q", tt.wantErr, err.Error())
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			switch tt.wantType {
			case "git":
				gitSrc, ok := src.(*GitBlueprintSource)
				if !ok {
					t.Fatalf("expected *GitBlueprintSource, got %T", src)
				}
				if gitSrc.RepoURL != tt.repo {
					t.Errorf("RepoURL = %q, want %q", gitSrc.RepoURL, tt.repo)
				}
				if gitSrc.Rev != tt.rev {
					t.Errorf("Rev = %q, want %q", gitSrc.Rev, tt.rev)
				}
				if gitSrc.Subdir != tt.subdir {
					t.Errorf("Subdir = %q, want %q", gitSrc.Subdir, tt.subdir)
				}
			case "local":
				localSrc, ok := src.(*LocalBlueprintSource)
				if !ok {
					t.Fatalf("expected *LocalBlueprintSource, got %T", src)
				}
				if localSrc.Subdir != tt.subdir {
					t.Errorf("Subdir = %q, want %q", localSrc.Subdir, tt.subdir)
				}
			default:
				t.Fatalf("unknown wantType %q", tt.wantType)
			}
		})
	}
}

func TestValidateSubdir(t *testing.T) {
	tests := []struct {
		subdir  string
		wantErr bool
	}{
		{"", false},
		{"monitors", false},
		{"monitors/cve-2025", false},
		{"./monitors", false},
		{"/absolute", true},
		{"..", true},
		{"../escape", true},
		{"foo/../..", true},
		{"foo/bar/../../..", true},
	}

	for _, tt := range tests {
		t.Run(tt.subdir, func(t *testing.T) {
			err := validateSubdir(tt.subdir)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateSubdir(%q) error = %v, wantErr = %v", tt.subdir, err, tt.wantErr)
			}
		})
	}
}
