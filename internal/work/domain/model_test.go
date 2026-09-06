package domain

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestProjectIDValidation(t *testing.T) {
	id := NewProjectID()
	if err := id.Validate(); err != nil {
		t.Fatalf("NewProjectID().Validate() error = %v", err)
	}
	if err := ProjectID("0191a6c0-0000-7000-8000-000000000000").Validate(); err != nil {
		t.Fatalf("canonical UUIDv7 validation error = %v", err)
	}
	if err := ProjectID("0191a6c0-0000-4000-8000-000000000000").Validate(); !errors.Is(err, ErrManifestInvalid) {
		t.Fatalf("UUIDv4 validation error = %v, want ErrManifestInvalid", err)
	}
	if err := ProjectID("0191a6c0-0000-7000-c000-000000000000").Validate(); !errors.Is(err, ErrManifestInvalid) {
		t.Fatalf("non-RFC UUIDv7 validation error = %v, want ErrManifestInvalid", err)
	}
}

func TestRepositoryBindingValidation(t *testing.T) {
	binding := RepositoryBinding{Root: filepath.Join("/tmp", "repo"), ManifestPath: "/tmp/repo/.keystone/project.yaml"}
	if err := binding.Validate(); err != nil {
		t.Fatalf("valid binding error = %v", err)
	}
	if err := (RepositoryBinding{Root: "/tmp/../repo", ManifestPath: "/tmp/repo/.keystone/project.yaml"}).Validate(); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("unclean binding error = %v, want ErrInvalidRequest", err)
	}
}

func TestChangeIntentPreservesOriginalAndBoundsSummaryByRunes(t *testing.T) {
	original := "  中文\tintent\n" + strings.Repeat("界", 300)
	intent, err := NewChangeIntent(original)
	if err != nil {
		t.Fatal(err)
	}
	if intent.Original != original {
		t.Fatalf("original = %q, want %q", intent.Original, original)
	}
	if len([]rune(intent.Summary)) != 256 || !strings.HasPrefix(intent.Summary, "中文 intent ") {
		t.Fatalf("summary = %q", intent.Summary)
	}
	if err := intent.Validate(); err != nil {
		t.Fatal(err)
	}
	intent.Summary = "tampered"
	if !errors.Is(intent.Validate(), ErrInvalidRequest) {
		t.Fatal("tampered summary was accepted")
	}
}

func TestChangeLifecycleTransitionsAndLateResultFence(t *testing.T) {
	change := validChange(t)
	paused, err := change.Transition("pause")
	if err != nil || paused.Status != ChangeStatusPaused || paused.Version != 2 {
		t.Fatalf("pause = %+v, err = %v", paused, err)
	}
	if _, err := paused.Transition("pause"); !errors.Is(err, ErrLifecycleTransitionInvalid) {
		t.Fatalf("second pause error = %v", err)
	}
	resumed, err := paused.Transition("resume")
	if err != nil || resumed.Status != ChangeStatusActive || resumed.Version != 3 {
		t.Fatalf("resume = %+v, err = %v", resumed, err)
	}
	human, err := resumed.EnterHumanRequired()
	if err != nil || human.Status != ChangeStatusHumanRequired || human.Version != 4 {
		t.Fatalf("human required = %+v, err = %v", human, err)
	}
	retried, err := human.ApplyHumanDecision(HumanDecisionRetry)
	if err != nil || retried.Status != ChangeStatusActive || retried.Version != 5 {
		t.Fatalf("retry = %+v, err = %v", retried, err)
	}
	cancelled, err := retried.Transition("cancel")
	if err != nil || cancelled.Status != ChangeStatusCancelled || cancelled.Version != 6 {
		t.Fatalf("cancel = %+v, err = %v", cancelled, err)
	}
	if cancelled.CanAdvanceLateResult() {
		t.Fatal("cancelled change accepted a late result")
	}
}

func TestAgentRunCompletionIsOneTime(t *testing.T) {
	now := time.Now().UTC()
	run := AgentRun{ID: AgentRunID(NewProjectID()), ChangeID: ChangeID(NewProjectID()), Stage: LifecycleStageIntent, Attempt: 1, Status: AgentRunStatusRunning, StartedAt: now}
	if err := run.Validate(); err != nil {
		t.Fatal(err)
	}
	if err := run.Complete(AgentRunOutcomeFailed, now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if run.Outcome != AgentRunOutcomeFailed || run.CompletedAt == nil {
		t.Fatalf("completed run = %+v", run)
	}
	if !errors.Is(run.Complete(AgentRunOutcomeSucceeded, now.Add(2*time.Second)), ErrInvalidRequest) {
		t.Fatal("completed AgentRun accepted a second completion")
	}
}

func validChange(t *testing.T) Change {
	t.Helper()
	changeID := ChangeID(NewProjectID())
	intent := ArtifactRef{ID: ArtifactRefID(NewProjectID()), ChangeID: changeID, ArtifactID: ArtifactID(NewProjectID()), Role: ArtifactRoleChangeIntent, Ordinal: 0}
	change, err := NewChange(NewProjectID(), changeID, "/tmp/repo", strings.Repeat("a", 40), intent, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	return change
}
