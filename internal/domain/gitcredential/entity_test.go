package gitcredential

import "testing"

func TestNormalizeBaseURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "trims trailing slash", raw: "http://git.cloud.local:9080/", want: "http://git.cloud.local:9080"},
		{name: "adds default scheme", raw: "git.cloud.local:9080", want: "http://git.cloud.local:9080"},
		{name: "lowercases scheme and host", raw: "HTTP://Git.Cloud.Local:9080", want: "http://git.cloud.local:9080"},
		{name: "keeps path case", raw: "http://git.cloud.local:9080/code/bigData/", want: "http://git.cloud.local:9080/code/bigData"},
		{name: "drops query and fragment", raw: "http://git.cloud.local:9080/code/app.git?ref=main#top", want: "http://git.cloud.local:9080/code/app.git"},
		{name: "keeps https", raw: "https://git.example.com/", want: "https://git.example.com"},
		{name: "blank stays blank", raw: "   ", want: ""},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := NormalizeBaseURL(test.raw); got != test.want {
				t.Fatalf("NormalizeBaseURL(%q) = %q, want %q", test.raw, got, test.want)
			}
		})
	}
}

func TestRepoPathFromURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		repoURL string
		want    string
	}{
		{
			name:    "cloud.local gitlab with port",
			repoURL: "http://git.cloud.local:9080/code/bigData/fusion-source-web.git",
			want:    "code/bigData/fusion-source-web",
		},
		{
			name:    "ip address gitlab with port",
			repoURL: "http://192.168.2.34:9080/code/bigData/fusion-source-web.git",
			want:    "code/bigData/fusion-source-web",
		},
		{name: "without .git suffix", repoURL: "https://gitlab.example.com/group/sub/repo", want: "group/sub/repo"},
		{name: "scp like syntax", repoURL: "git@gitlab.example.com:code/bigData/fusion-source-web.git", want: "code/bigData/fusion-source-web"},
		{name: "drops query", repoURL: "http://git.cloud.local:9080/code/app.git?ref=main", want: "code/app"},
		{name: "host only", repoURL: "http://git.cloud.local:9080/", want: ""},
		{name: "host without path", repoURL: "http://git.cloud.local:9080", want: ""},
		{name: "blank", repoURL: "  ", want: ""},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := RepoPathFromURL(test.repoURL); got != test.want {
				t.Fatalf("RepoPathFromURL(%q) = %q, want %q", test.repoURL, got, test.want)
			}
		})
	}
}

func TestResolveForRepoURLPrefersLongestPrefix(t *testing.T) {
	t.Parallel()

	hostWide := Credential{ID: "gc-host", BaseURL: "http://git.cloud.local:9080"}
	groupScoped := Credential{ID: "gc-group", BaseURL: "http://git.cloud.local:9080/code/bigData"}
	ipHost := Credential{ID: "gc-ip", BaseURL: "http://192.168.2.34:9080"}
	projectScoped := Credential{ID: "gc-project", BaseURL: "http://git.cloud.local:9080/code/bigData/fusion-source-web"}
	credentials := []Credential{hostWide, groupScoped, ipHost}

	tests := []struct {
		name    string
		repoURL string
		wantID  string
		wantOK  bool
	}{
		{
			name:    "group credential beats host credential",
			repoURL: "http://git.cloud.local:9080/code/bigData/fusion-source-web.git",
			wantID:  "gc-group",
			wantOK:  true,
		},
		{
			name:    "host credential matches a project outside the group",
			repoURL: "http://git.cloud.local:9080/other/app.git",
			wantID:  "gc-host",
			wantOK:  true,
		},
		{
			name:    "ip host credential matches its own address",
			repoURL: "http://192.168.2.34:9080/code/bigData/fusion-source-web.git",
			wantID:  "gc-ip",
			wantOK:  true,
		},
		{
			name:    "scheme and host are compared case insensitively",
			repoURL: "HTTP://GIT.CLOUD.LOCAL:9080/code/bigData/app.git",
			wantID:  "gc-group",
			wantOK:  true,
		},
		{
			name:    "unknown host has no credential",
			repoURL: "http://git.other.local:9080/code/app.git",
			wantOK:  false,
		},
		{
			name:    "blank repository url has no credential",
			repoURL: "   ",
			wantOK:  false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got, ok := ResolveForRepoURL(credentials, test.repoURL)
			if ok != test.wantOK {
				t.Fatalf("ResolveForRepoURL(%q) ok = %v, want %v", test.repoURL, ok, test.wantOK)
			}
			if ok && got.ID != test.wantID {
				t.Fatalf("ResolveForRepoURL(%q) id = %q, want %q", test.repoURL, got.ID, test.wantID)
			}
			if !ok && got.ID != "" {
				t.Fatalf("ResolveForRepoURL returned credential %q for an unmatched url", got.ID)
			}
		})
	}

	t.Run("project scoped credential matches the clone url", func(t *testing.T) {
		t.Parallel()
		got, ok := ResolveForRepoURL([]Credential{hostWide, projectScoped}, "http://git.cloud.local:9080/code/bigData/fusion-source-web.git")
		if !ok || got.ID != "gc-project" {
			t.Fatalf("resolve = %q ok=%v, want gc-project", got.ID, ok)
		}
	})

	t.Run("path prefix must end on a separator", func(t *testing.T) {
		t.Parallel()
		codeScoped := Credential{ID: "gc-code", BaseURL: "http://git.cloud.local:9080/code"}
		if got, ok := ResolveForRepoURL([]Credential{codeScoped}, "http://git.cloud.local:9080/codebase/app.git"); ok {
			t.Fatalf("ResolveForRepoURL matched %q for /codebase/app.git", got.ID)
		}
		if got, ok := ResolveForRepoURL([]Credential{codeScoped}, "http://git.cloud.local:9080/code/app.git"); !ok || got.ID != "gc-code" {
			t.Fatalf("ResolveForRepoURL = %q ok=%v, want gc-code", got.ID, ok)
		}
	})

	t.Run("no credentials", func(t *testing.T) {
		t.Parallel()
		if _, ok := ResolveForRepoURL(nil, "http://git.cloud.local:9080/code/app.git"); ok {
			t.Fatalf("ResolveForRepoURL matched without any credential")
		}
	})
}
