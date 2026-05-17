// Author: Kaviru Hapuarachchi
// GitHub: https://github.com/kavirubc
// Created: 2026-02-02
// Last Modified: 2026-02-02

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/similigh/simili-bot/internal/core/config"
	"github.com/similigh/simili-bot/internal/core/pipeline"
	"github.com/similigh/simili-bot/internal/steps"
)

// MockStep mocks the pipeline.Step interface.
// This is provided for future test scenarios where we need to mock specific steps.
// Currently, the E2E test uses real pipeline steps to verify end-to-end behavior.
type MockStep struct {
	NameFunc func() string
	RunFunc  func(ctx *pipeline.Context) error
}

func (m *MockStep) Name() string {
	if m.NameFunc != nil {
		return m.NameFunc()
	}
	return "mock_step"
}

func (m *MockStep) Run(ctx *pipeline.Context) error {
	if m.RunFunc != nil {
		return m.RunFunc(ctx)
	}
	return nil
}

// TestPRCollectionConfigWiring verifies that the pr_collection field is correctly
// parsed and validated through the config layer without requiring a live Qdrant instance.
func TestPRCollectionConfigWiring(t *testing.T) {
	cfg := &config.Config{
		Qdrant: config.QdrantConfig{
			URL:          "https://example.qdrant.io:6334",
			APIKey:       "qdrant-key",
			Collection:   "simili_bot_v1",
			PRCollection: "simili_prs_v1",
		},
		Embedding: config.EmbeddingConfig{
			APIKey: "embedding-key",
		},
	}

	if err := cfg.Validate(); err != nil {
		t.Fatalf("Config with pr_collection should be valid: %v", err)
	}
	if cfg.Qdrant.PRCollection != "simili_prs_v1" {
		t.Errorf("Expected PRCollection 'simili_prs_v1', got %q", cfg.Qdrant.PRCollection)
	}
}

func TestEndToEndPipeline(t *testing.T) {
	// 1. Setup minimal config and issue
	cfg := &config.Config{
		Defaults: config.DefaultsConfig{
			SimilarityThreshold: 0.8,
			MaxSimilarToShow:    3,
			DuplicateCandidates: 5,
		},
	}

	issue := &pipeline.Issue{
		Org:    "test-org",
		Repo:   "test-repo",
		Number: 1337,
		Title:  "Integration Test Issue",
		Body:   "This is a test issue for E2E verification.",
		State:  "open",
	}

	ctx := context.Background()
	pCtx := pipeline.NewContext(ctx, issue, cfg)

	// 2. Setup mock dependencies (we invoke "mock-clients" via DryRun for now)
	deps := &pipeline.Dependencies{
		DryRun: true,
	}

	// 3. Create pipeline using Registry
	registry := pipeline.NewRegistry()
	steps.RegisterAll(registry)

	// Use the "issue-triage" preset
	// Note: In real E2E we would want real integrations, but for CI/basic verify here, we check plumbing.
	stepNames := pipeline.ResolveSteps(nil, "issue-triage")

	p, err := registry.BuildFromNames(stepNames, deps)
	if err != nil {
		t.Fatalf("Failed to build pipeline: %v", err)
	}

	// 4. Run Pipeline
	startTime := time.Now()
	err = p.Run(pCtx)
	duration := time.Since(startTime)

	// 5. Verify Results
	if err != nil {
		t.Fatalf("Pipeline execution failed: %v", err)
	}

	t.Logf("Pipeline passed in %v", duration)
	t.Logf("Result: %+v", pCtx.Result)

	// In "DryRun" mode on "triage" step (if implemented to skip LLM or use mock),
	// we might not get suggested labels if the step requires real LLM.
	// But "gatekeeper" should have passed.

	if pCtx.Result.Skipped {
		t.Logf("Pipeline skipped: %s", pCtx.Result.SkipReason)
	} else {
		// If not skipped, check basics
		if pCtx.Result.IssueNumber != 1337 {
			t.Errorf("Expected issue number 1337, got %d", pCtx.Result.IssueNumber)
		}
	}
}

// TestEndToEndPipeline_GitHubNative exercises the "issue-triage-github" preset
// which uses the github_native search backend (no Qdrant, no embedding key).
// This ensures the zero-config deployment path builds and runs without errors.
func TestEndToEndPipeline_GitHubNative(t *testing.T) {
	cfg := &config.Config{
		Search: config.SearchConfig{
			Backend: "github_native",
		},
		Defaults: config.DefaultsConfig{
			SimilarityThreshold: 0.15,
			MaxSimilarToShow:    5,
			DuplicateCandidates: 5,
		},
	}
	cfg.ApplyDefaults()

	issue := &pipeline.Issue{
		Org:    "test-org",
		Repo:   "test-repo",
		Number: 42,
		Title:  "GitHub Native Pipeline Test Issue",
		Body:   "Verify the github_native preset pipeline runs end-to-end in DryRun mode.",
		State:  "open",
	}

	ctx := context.Background()
	pCtx := pipeline.NewContext(ctx, issue, cfg)

	deps := &pipeline.Dependencies{
		DryRun: true,
	}

	registry := pipeline.NewRegistry()
	steps.RegisterAll(registry)

	// Use the "issue-triage-github" preset — the github_native equivalent
	stepNames := pipeline.ResolveSteps(nil, "issue-triage-github")

	p, err := registry.BuildFromNames(stepNames, deps)
	if err != nil {
		t.Fatalf("Failed to build github_native pipeline: %v", err)
	}

	startTime := time.Now()
	err = p.Run(pCtx)
	duration := time.Since(startTime)

	if err != nil {
		t.Fatalf("GitHub-native pipeline execution failed: %v", err)
	}

	t.Logf("GitHub-native pipeline passed in %v", duration)
	t.Logf("Result: %+v", pCtx.Result)

	if pCtx.Result.Skipped {
		t.Logf("Pipeline skipped: %s", pCtx.Result.SkipReason)
	} else {
		if pCtx.Result.IssueNumber != 42 {
			t.Errorf("Expected issue number 42, got %d", pCtx.Result.IssueNumber)
		}
	}
}

// TestGitHubNativeConfigValidation verifies that config validation passes for
// the github_native backend when Qdrant fields are completely empty.
func TestGitHubNativeConfigValidation(t *testing.T) {
	cfg := &config.Config{
		Search: config.SearchConfig{
			Backend: "github_native",
		},
	}
	cfg.ApplyDefaults()

	if err := cfg.Validate(); err != nil {
		t.Fatalf("github_native config should validate without Qdrant fields: %v", err)
	}

	// Verify Qdrant fields are empty and that's OK
	if cfg.Qdrant.URL != "" || cfg.Qdrant.APIKey != "" || cfg.Qdrant.Collection != "" || cfg.Qdrant.PRCollection != "" {
		t.Error("Expected empty Qdrant fields for github_native backend")
	}

	// Verify the backend was set correctly
	if cfg.Search.Backend != "github_native" {
		t.Errorf("Expected search.backend 'github_native', got %q", cfg.Search.Backend)
	}
}
