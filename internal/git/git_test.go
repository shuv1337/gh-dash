package git

import (
	"strings"
	"testing"
)

func TestParseGitHubRepoFromUrl(t *testing.T) {
	tests := []struct {
		name        string
		url         string
		wantOwner   string
		wantName    string
		wantErr     bool
		errContains string
	}{
		// Standard github.com HTTPS URLs
		{
			name:      "github.com HTTPS with .git suffix",
			url:       "https://github.com/owner/repo.git",
			wantOwner: "owner",
			wantName:  "repo",
		},
		{
			name:      "github.com HTTPS without .git suffix",
			url:       "https://github.com/owner/repo",
			wantOwner: "owner",
			wantName:  "repo",
		},
		{
			name:      "github.com HTTPS with trailing slash",
			url:       "https://github.com/owner/repo/",
			wantOwner: "owner",
			wantName:  "repo",
		},
		// Standard github.com SSH URLs
		{
			name:      "github.com SSH with .git suffix",
			url:       "git@github.com:owner/repo.git",
			wantOwner: "owner",
			wantName:  "repo",
		},
		{
			name:      "github.com SSH without .git suffix",
			url:       "git@github.com:owner/repo",
			wantOwner: "owner",
			wantName:  "repo",
		},
		// GitHub Enterprise HTTPS URLs
		{
			name:      "GitHub Enterprise HTTPS",
			url:       "https://github.enterprise.com/owner/repo.git",
			wantOwner: "owner",
			wantName:  "repo",
		},
		{
			name:      "GitHub Enterprise HTTPS without .git",
			url:       "https://github.mycompany.com/org/project",
			wantOwner: "org",
			wantName:  "project",
		},
		{
			name:      "GitHub Enterprise HTTPS with subdomain",
			url:       "https://git.internal.company.io/team/service",
			wantOwner: "team",
			wantName:  "service",
		},
		// GitHub Enterprise SSH URLs
		{
			name:      "GitHub Enterprise SSH",
			url:       "git@github.enterprise.com:owner/repo.git",
			wantOwner: "owner",
			wantName:  "repo",
		},
		{
			name:      "GitHub Enterprise SSH without .git",
			url:       "git@github.mycompany.com:org/project",
			wantOwner: "org",
			wantName:  "project",
		},
		{
			name:      "GitHub Enterprise SSH with subdomain",
			url:       "git@git.internal.company.io:team/service.git",
			wantOwner: "team",
			wantName:  "service",
		},
		// HTTP URLs (some self-hosted instances use HTTP)
		{
			name:      "HTTP URL",
			url:       "http://github.local/owner/repo.git",
			wantOwner: "owner",
			wantName:  "repo",
		},
		// Edge cases with whitespace
		{
			name:      "URL with leading/trailing whitespace",
			url:       "  https://github.com/owner/repo.git  ",
			wantOwner: "owner",
			wantName:  "repo",
		},
		// Error cases
		{
			name:        "empty URL",
			url:         "",
			wantErr:     true,
			errContains: "unsupported URL format",
		},
		{
			name:        "unsupported protocol",
			url:         "ftp://github.com/owner/repo",
			wantErr:     true,
			errContains: "unsupported URL format",
		},
		{
			name:        "SSH URL missing colon",
			url:         "git@github.com/owner/repo",
			wantErr:     true,
			errContains: "missing colon separator",
		},
		{
			name:        "HTTPS URL with only owner",
			url:         "https://github.com/owner",
			wantErr:     true,
			errContains: "expected owner/repo",
		},
		{
			name:        "SSH URL with only owner",
			url:         "git@github.com:owner",
			wantErr:     true,
			errContains: "expected owner/repo",
		},
		{
			name:        "HTTPS URL with too many path segments",
			url:         "https://github.com/owner/repo/extra/path",
			wantErr:     true,
			errContains: "expected owner/repo",
		},
		{
			name:        "SSH URL with empty owner",
			url:         "git@github.com:/repo",
			wantErr:     true,
			errContains: "expected owner/repo",
		},
		{
			name:        "HTTPS URL with empty repo",
			url:         "https://github.com/owner/",
			wantErr:     true,
			errContains: "expected owner/repo",
		},
		{
			name:        "HTTPS URL with no path",
			url:         "https://github.com",
			wantErr:     true,
			errContains: "missing path",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			owner, name, err := ParseGitHubRepoFromUrl(tt.url)

			if tt.wantErr {
				if err == nil {
					t.Errorf("ParseGitHubRepoFromUrl(%q) expected error containing %q, got nil", tt.url, tt.errContains)
					return
				}
				if tt.errContains != "" && !contains(err.Error(), tt.errContains) {
					t.Errorf("ParseGitHubRepoFromUrl(%q) error = %q, want error containing %q", tt.url, err.Error(), tt.errContains)
				}
				return
			}

			if err != nil {
				t.Errorf("ParseGitHubRepoFromUrl(%q) unexpected error: %v", tt.url, err)
				return
			}

			if owner != tt.wantOwner {
				t.Errorf("ParseGitHubRepoFromUrl(%q) owner = %q, want %q", tt.url, owner, tt.wantOwner)
			}
			if name != tt.wantName {
				t.Errorf("ParseGitHubRepoFromUrl(%q) name = %q, want %q", tt.url, name, tt.wantName)
			}
		})
	}
}

func TestGetRepoShortName(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want string
	}{
		{
			name: "standard github URL with .git",
			url:  "https://github.com/owner/repo.git",
			want: "owner/repo",
		},
		{
			name: "standard github URL without .git",
			url:  "https://github.com/owner/repo",
			want: "owner/repo",
		},
		{
			name: "non-github URL",
			url:  "https://gitlab.com/owner/repo",
			want: "https://gitlab.com/owner/repo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetRepoShortName(tt.url)
			if got != tt.want {
				t.Errorf("GetRepoShortName(%q) = %q, want %q", tt.url, got, tt.want)
			}
		})
	}
}

// contains checks if s contains substr (case-sensitive)
func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}
