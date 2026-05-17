package commands

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	githubapi "github.com/google/go-github/v60/github"
	"github.com/similigh/simili-bot/internal/core/pipeline"
)

func TestEnrichIssueFromGitHubEvent_PullRequest(t *testing.T) {
	issue := &pipeline.Issue{}
	raw := map[string]interface{}{
		"action": "opened",
		"pull_request": map[string]interface{}{
			"number":     float64(42),
			"title":      "feat: add PR support",
			"body":       "Implements PR pipeline support",
			"state":      "open",
			"html_url":   "https://github.com/similigh/simili-bot/pull/42",
			"created_at": "2026-02-13T00:00:00Z",
			"user": map[string]interface{}{
				"login": "contributor",
			},
			"labels": []interface{}{
				map[string]interface{}{"name": "enhancement"},
				map[string]interface{}{"name": "ai"},
			},
		},
		"repository": map[string]interface{}{
			"name": "simili-bot",
			"owner": map[string]interface{}{
				"login": "similigh",
			},
		},
	}

	enrichIssueFromGitHubEvent(issue, raw)

	if issue.EventType != "pull_request" {
		t.Fatalf("expected pull_request event type, got %q", issue.EventType)
	}
	if issue.EventAction != "opened" {
		t.Fatalf("expected opened action, got %q", issue.EventAction)
	}
	if issue.Number != 42 || issue.Org != "similigh" || issue.Repo != "simili-bot" {
		t.Fatalf("unexpected issue identity: %+v", issue)
	}
	if issue.URL == "" || issue.Author != "contributor" || issue.State != "open" {
		t.Fatalf("expected PR fields to be parsed, got %+v", issue)
	}
	if len(issue.Labels) != 2 {
		t.Fatalf("expected labels to be parsed, got %+v", issue.Labels)
	}
	if issue.CreatedAt.IsZero() || !issue.CreatedAt.Equal(time.Date(2026, 2, 13, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("expected created_at to be parsed, got %v", issue.CreatedAt)
	}
}

func TestEnrichIssueFromGitHubEvent_IssueComment(t *testing.T) {
	issue := &pipeline.Issue{}
	raw := map[string]interface{}{
		"comment": map[string]interface{}{
			"body": "/undo",
			"user": map[string]interface{}{"login": "maintainer"},
		},
		"issue": map[string]interface{}{
			"number": float64(15),
			"title":  "Bug report",
			"body":   "Details",
		},
	}

	enrichIssueFromGitHubEvent(issue, raw)

	if issue.EventType != "issue_comment" {
		t.Fatalf("expected issue_comment event type, got %q", issue.EventType)
	}
	if issue.CommentBody != "/undo" || issue.CommentAuthor != "maintainer" {
		t.Fatalf("expected comment data, got %+v", issue)
	}
	if issue.Number != 15 {
		t.Fatalf("expected related issue number to be parsed, got %d", issue.Number)
	}
}

func TestEnrichIssueFromGitHubEvent_PRComment(t *testing.T) {
	issue := &pipeline.Issue{}
	raw := map[string]interface{}{
		"action": "created",
		"comment": map[string]interface{}{
			"body": "Looks good!",
			"user": map[string]interface{}{"login": "reviewer"},
		},
		"issue": map[string]interface{}{
			"number":       float64(42),
			"title":        "feat: add PR support",
			"body":         "PR description",
			"pull_request": map[string]interface{}{"url": "https://api.github.com/repos/org/repo/pulls/42"},
		},
		"repository": map[string]interface{}{
			"name":  "simili-bot",
			"owner": map[string]interface{}{"login": "similigh"},
		},
	}

	enrichIssueFromGitHubEvent(issue, raw)

	if issue.EventType != "pr_comment" {
		t.Fatalf("expected pr_comment event type, got %q", issue.EventType)
	}
	if issue.CommentBody != "Looks good!" || issue.CommentAuthor != "reviewer" {
		t.Fatalf("expected comment data, got body=%q author=%q", issue.CommentBody, issue.CommentAuthor)
	}
	if issue.Number != 42 {
		t.Fatalf("expected issue number 42, got %d", issue.Number)
	}
}

func TestGithubIssueToPipelineIssue(t *testing.T) {
	createdAt := githubapi.Timestamp{Time: time.Date(2026, 2, 13, 10, 0, 0, 0, time.UTC)}
	ghIssue := &githubapi.Issue{
		Number:    githubapi.Int(17),
		Title:     githubapi.String("feat: fetch issue directly"),
		Body:      githubapi.String("Implement CLI support"),
		State:     githubapi.String("open"),
		HTMLURL:   githubapi.String("https://github.com/similigh/simili-bot/issues/17"),
		CreatedAt: &createdAt,
		User:      &githubapi.User{Login: githubapi.String("maintainer")},
		Labels: []*githubapi.Label{
			{Name: githubapi.String("enhancement")},
			{Name: githubapi.String("cli")},
		},
	}

	issue := githubIssueToPipelineIssue(ghIssue, "similigh", "simili-bot")

	if issue.Org != "similigh" || issue.Repo != "simili-bot" || issue.Number != 17 {
		t.Fatalf("unexpected issue identity: %+v", issue)
	}
	if issue.Title != "feat: fetch issue directly" || issue.Body != "Implement CLI support" {
		t.Fatalf("unexpected title/body: %+v", issue)
	}
	if issue.State != "open" || issue.Author != "maintainer" || issue.URL == "" {
		t.Fatalf("expected state/author/url parsed, got %+v", issue)
	}
	if len(issue.Labels) != 2 || issue.Labels[0] != "enhancement" || issue.Labels[1] != "cli" {
		t.Fatalf("unexpected labels: %+v", issue.Labels)
	}
	if !issue.CreatedAt.Equal(createdAt.Time) {
		t.Fatalf("expected created_at to be parsed, got %v", issue.CreatedAt)
	}
	if issue.EventType != "issues" || issue.EventAction != "opened" {
		t.Fatalf("expected issues/opened event, got %s/%s", issue.EventType, issue.EventAction)
	}
}

func TestGithubIssueToPipelineIssue_NilIssue(t *testing.T) {
	issue := githubIssueToPipelineIssue(nil, "similigh", "simili-bot")

	if issue.Org != "similigh" || issue.Repo != "simili-bot" {
		t.Fatalf("unexpected org/repo for nil issue: %+v", issue)
	}
	if issue.EventType != "issues" || issue.EventAction != "opened" {
		t.Fatalf("expected default issues/opened for nil issue, got %s/%s", issue.EventType, issue.EventAction)
	}
}

func TestResolveIssueRepo(t *testing.T) {
	tests := []struct {
		name     string
		flagOrg  string
		flagRepo string
		envRepo  string
		wantOrg  string
		wantRepo string
	}{
		{
			name:     "combined owner/repo in --repo flag",
			flagRepo: "similigh/simili-bot",
			wantOrg:  "similigh",
			wantRepo: "simili-bot",
		},
		{
			name:     "separate --org and --repo flags",
			flagOrg:  "similigh",
			flagRepo: "simili-bot",
			wantOrg:  "similigh",
			wantRepo: "simili-bot",
		},
		{
			name:     "GITHUB_REPOSITORY env fallback",
			envRepo:  "similigh/simili-bot",
			wantOrg:  "similigh",
			wantRepo: "simili-bot",
		},
		{
			name:     "--repo flag takes precedence over env",
			flagRepo: "other/repo",
			envRepo:  "similigh/simili-bot",
			wantOrg:  "other",
			wantRepo: "repo",
		},
		{
			name:     "empty flags and no env returns empty",
			wantOrg:  "",
			wantRepo: "",
		},
		{
			name:     "only --repo without slash and no --org returns empty",
			flagRepo: "simili-bot",
			wantOrg:  "",
			wantRepo: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envRepo != "" {
				t.Setenv("GITHUB_REPOSITORY", tt.envRepo)
			} else {
				t.Setenv("GITHUB_REPOSITORY", "")
			}

			org, repo := resolveIssueRepo(tt.flagOrg, tt.flagRepo)
			if org != tt.wantOrg || repo != tt.wantRepo {
				t.Errorf("resolveIssueRepo(%q, %q) = (%q, %q), want (%q, %q)",
					tt.flagOrg, tt.flagRepo, org, repo, tt.wantOrg, tt.wantRepo)
			}
		})
	}
}

func TestPopulateIssuePayload(t *testing.T) {
	issue := &pipeline.Issue{}
	payload := map[string]any{
		"number":     float64(99),
		"title":      "Test issue",
		"body":       "Issue body",
		"state":      "open",
		"html_url":   "https://github.com/org/repo/issues/99",
		"created_at": "2026-01-15T12:00:00Z",
		"user":       map[string]any{"login": "testuser"},
		"labels": []any{
			map[string]any{"name": "bug"},
			map[string]any{"name": "help wanted"},
		},
	}

	populateIssuePayload(issue, payload)

	if issue.Number != 99 {
		t.Errorf("Number = %d, want 99", issue.Number)
	}
	if issue.Title != "Test issue" {
		t.Errorf("Title = %q, want %q", issue.Title, "Test issue")
	}
	if issue.Body != "Issue body" {
		t.Errorf("Body = %q, want %q", issue.Body, "Issue body")
	}
	if issue.State != "open" {
		t.Errorf("State = %q, want open", issue.State)
	}
	if issue.URL != "https://github.com/org/repo/issues/99" {
		t.Errorf("URL = %q, unexpected", issue.URL)
	}
	if issue.Author != "testuser" {
		t.Errorf("Author = %q, want testuser", issue.Author)
	}
	if !issue.CreatedAt.Equal(time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)) {
		t.Errorf("CreatedAt = %v, unexpected", issue.CreatedAt)
	}
	if len(issue.Labels) != 2 || issue.Labels[0] != "bug" || issue.Labels[1] != "help wanted" {
		t.Errorf("Labels = %v, unexpected", issue.Labels)
	}
}

func TestPopulateIssuePayload_EmptyLabelsNotOverwrite(t *testing.T) {
	issue := &pipeline.Issue{Labels: []string{"existing"}}
	// payload with no labels key — existing labels should be preserved
	populateIssuePayload(issue, map[string]any{"number": float64(1)})
	if len(issue.Labels) != 1 || issue.Labels[0] != "existing" {
		t.Errorf("Labels overwritten unexpectedly: %v", issue.Labels)
	}
}

// captureStderr redirects os.Stderr for the duration of fn and returns what was written.
func captureStderr(fn func()) string {
	r, w, err := os.Pipe()
	if err != nil {
		panic("captureStderr: os.Pipe: " + err.Error())
	}
	old := os.Stderr
	os.Stderr = w
	defer func() {
		os.Stderr = old
		r.Close()
	}()

	func() {
		defer w.Close()
		fn()
	}()

	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	return buf.String()
}

func TestWarningToStderr_ConfigLoadFailure(t *testing.T) {
	// Write an invalid YAML config to a temp file to trigger the "Failed to load config" warning.
	f, err := os.CreateTemp(t.TempDir(), "bad-config-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	fmt.Fprintln(f, "invalid: [yaml: }{")
	f.Close()

	got := captureStderr(func() {
		_, _ = loadConfigWithWarning(f.Name(), nil)
	})

	if !strings.Contains(got, "Warning: Failed to load config") {
		t.Errorf("expected warning on stderr, got: %q", got)
	}
}
