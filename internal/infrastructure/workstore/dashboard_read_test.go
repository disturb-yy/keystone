package workstore

import (
	"context"
	"sort"
	"testing"

	"github.com/disturb-yy/keystone/internal/work"
	"github.com/disturb-yy/keystone/internal/work/domain"
)

func TestListProjectSummariesAndChangesUseOpaqueStableCursors(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	projects := make([]domain.Project, 0, 3)
	for index := 0; index < 3; index++ {
		root := "/tmp/dashboard-project-" + string(rune('a'+index))
		reservation, err := store.Reserve(ctx, root, "dashboard-project-key-"+string(rune('a'+index)), domain.NewProjectID())
		if err != nil {
			t.Fatal(err)
		}
		project, err := store.Finalize(ctx, "dashboard-project-key-"+string(rune('a'+index)), reservation.Intent, domain.ProjectManifest{Version: 1, ProjectID: reservation.Intent.ProjectID}, domain.RepositoryBinding{Root: root, ManifestPath: root + "/.keystone/project.yaml"}, "")
		if err != nil {
			t.Fatal(err)
		}
		projects = append(projects, project)
	}
	page, err := store.ListProjectSummaries(ctx, DashboardPage{Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Projects) != 2 || !page.HasMore || page.NextCursor == "" {
		t.Fatalf("first project page = %+v", page)
	}
	if page.NextCursor[0] == '{' {
		t.Fatalf("cursor should be opaque: %q", page.NextCursor)
	}
	second, err := store.ListProjectSummaries(ctx, DashboardPage{Limit: 2, Cursor: page.NextCursor})
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Projects) != 1 || second.HasMore {
		t.Fatalf("second project page = %+v", second)
	}
	allIDs := []string{string(page.Projects[0].ProjectID), string(page.Projects[1].ProjectID), string(second.Projects[0].ProjectID)}
	if !sort.StringsAreSorted(allIDs) {
		t.Fatalf("project IDs are not sorted: %v", allIDs)
	}

	intent, err := domain.NewChangeIntent("dashboard change")
	if err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 3; index++ {
		_, err := store.CreateChange(ctx, work.ChangeCreateRecord{Project: projects[0], Snapshot: domain.ChangeSourceSnapshot{RepositoryRoot: projects[0].Binding.Root, BaseRevision: "0123456789012345678901234567890123456789"}, Intent: intent, IntentIdentity: domain.NewArtifactIdentity([]byte(intent.Original)), IdempotencyKey: "dashboard-change-key-" + string(rune('a'+index)), Actor: "test"})
		if err != nil {
			t.Fatal(err)
		}
	}
	changes, err := store.ListProjectChanges(ctx, projects[0].Identity.ProjectID, DashboardPage{Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(changes.Changes) != 2 || !changes.HasMore || changes.NextCursor == "" {
		t.Fatalf("first changes page = %+v", changes)
	}
	nextChanges, err := store.ListProjectChanges(ctx, projects[0].Identity.ProjectID, DashboardPage{Limit: 2, Cursor: changes.NextCursor})
	if err != nil {
		t.Fatal(err)
	}
	if len(nextChanges.Changes) != 1 || nextChanges.HasMore {
		t.Fatalf("second changes page = %+v", nextChanges)
	}
	if _, err := store.ListProjectChanges(ctx, projects[1].Identity.ProjectID, DashboardPage{Limit: 2, Cursor: changes.NextCursor}); err == nil {
		t.Fatal("cross-project cursor unexpectedly accepted")
	}
}

func TestListNeedsHumanUsesDaemonMarkedStatusAndRequiredEvent(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	root := "/tmp/dashboard-needs-human"
	reservation, err := store.Reserve(ctx, root, "dashboard-needs-project", domain.NewProjectID())
	if err != nil {
		t.Fatal(err)
	}
	project, err := store.Finalize(ctx, "dashboard-needs-project", reservation.Intent, domain.ProjectManifest{Version: 1, ProjectID: reservation.Intent.ProjectID}, domain.RepositoryBinding{Root: root, ManifestPath: root + "/.keystone/project.yaml"}, "")
	if err != nil {
		t.Fatal(err)
	}
	intent, _ := domain.NewChangeIntent("needs human")
	change, err := store.CreateChange(ctx, work.ChangeCreateRecord{Project: project, Snapshot: domain.ChangeSourceSnapshot{RepositoryRoot: root, BaseRevision: "0123456789012345678901234567890123456789"}, Intent: intent, IntentIdentity: domain.NewArtifactIdentity([]byte(intent.Original)), IdempotencyKey: "dashboard-needs-change", Actor: "test"})
	if err != nil {
		t.Fatal(err)
	}
	run, err := store.StartAgentRunWithArtifacts(ctx, change.ID, "test", nil)
	if err != nil {
		t.Fatal(err)
	}
	failure := insertTestArtifactRef(t, store, project, change, domain.ArtifactRoleFailure, "failure")
	if _, err := store.CompleteAgentRunWithArtifacts(ctx, run.ID, domain.AgentRunOutcomeFailed, "test", []domain.AgentRunArtifact{{ArtifactRefID: failure.ID, Role: domain.ArtifactRoleFailure, Ordinal: 0}}); err != nil {
		t.Fatal(err)
	}
	page, err := store.ListNeedsHuman(ctx, project.Identity.ProjectID, "", DashboardPage{Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].Change.Status != domain.ChangeStatusHumanRequired || page.Items[0].ReasonCode != "change_human_required" || page.Items[0].RequiredAt.IsZero() {
		t.Fatalf("needs human page = %+v", page)
	}
	if len(page.Items[0].EvidenceArtifactRefs) != 1 || page.Items[0].EvidenceArtifactRefs[0] != failure.ID {
		t.Fatalf("needs human evidence = %+v", page.Items[0].EvidenceArtifactRefs)
	}
}
