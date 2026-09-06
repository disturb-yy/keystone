package work_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/disturb-yy/keystone/internal/work"
	"github.com/disturb-yy/keystone/internal/work/domain"
)

func TestChangeServiceCreatePersistsOriginalIntentAfterStableSnapshot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "repository")
	project := &domain.Project{Identity: domain.RepositoryIdentity{ProjectID: domain.NewProjectID()}, Binding: domain.RepositoryBinding{Root: root, ManifestPath: filepath.Join(root, ".keystone", "project.yaml")}, CreatedAt: time.Now().UTC()}
	intent, err := domain.NewChangeIntent("  preserve\tthis intent  ")
	if err != nil {
		t.Fatal(err)
	}
	identity := domain.NewArtifactIdentity([]byte(intent.Original))
	state := &changeServiceState{change: domain.Change{ID: domain.ChangeID(domain.NewProjectID())}}
	artifacts := &changeServiceArtifacts{identity: identity}
	snapshot := &changeServiceSnapshot{snapshot: domain.ChangeSourceSnapshot{RepositoryRoot: root, BaseRevision: "0123456789012345678901234567890123456789"}}
	service, err := work.NewChangeService(changeServiceProjects{project: project}, snapshot, artifacts, state)
	if err != nil {
		t.Fatal(err)
	}
	created, err := service.Create(context.Background(), work.ChangeCreateRequest{RepositoryPath: root, Intent: intent.Original, IdempotencyKey: "create-key", Actor: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if created.ID != state.change.ID || state.record.Intent.Original != intent.Original || state.record.Intent.Summary != intent.Summary {
		t.Fatalf("created = %+v, record = %+v", created, state.record)
	}
	if snapshot.calls != 1 || artifacts.putCalls != 1 || state.createCalls != 1 {
		t.Fatalf("calls = snapshot:%d artifact:%d state:%d", snapshot.calls, artifacts.putCalls, state.createCalls)
	}
}

func TestChangeServiceCreateReplaysBeforeReadingCurrentRepository(t *testing.T) {
	root := filepath.Join(t.TempDir(), "repository")
	project := &domain.Project{Identity: domain.RepositoryIdentity{ProjectID: domain.NewProjectID()}, Binding: domain.RepositoryBinding{Root: root, ManifestPath: filepath.Join(root, ".keystone", "project.yaml")}, CreatedAt: time.Now().UTC()}
	changeID := domain.ChangeID(domain.NewProjectID())
	intent := "same intent"
	replay := domain.Change{ID: changeID, ProjectID: project.Identity.ProjectID, RepositoryRoot: root, Stage: domain.LifecycleStageIntent, Status: domain.ChangeStatusActive, Version: 1, BaseRevision: "0123456789012345678901234567890123456789", Intent: domain.ArtifactRef{ID: domain.ArtifactRefID(domain.NewProjectID()), ChangeID: changeID, ArtifactID: domain.ArtifactID(domain.NewProjectID()), Role: domain.ArtifactRoleChangeIntent}, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	state := &changeServiceState{change: replay, replay: true}
	snapshot := &changeServiceSnapshot{err: errors.New("repository is dirty")}
	artifacts := &changeServiceArtifacts{}
	service, err := work.NewChangeService(changeServiceProjects{project: project}, snapshot, artifacts, state)
	if err != nil {
		t.Fatal(err)
	}
	got, err := service.Create(context.Background(), work.ChangeCreateRequest{RepositoryPath: root, Intent: intent, IdempotencyKey: "replay-key"})
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != changeID || snapshot.calls != 0 || artifacts.putCalls != 0 || state.createCalls != 0 {
		t.Fatalf("replay = %+v, calls snapshot:%d artifact:%d state:%d", got, snapshot.calls, artifacts.putCalls, state.createCalls)
	}
}

type changeServiceProjects struct{ project *domain.Project }

func (p changeServiceProjects) FindProjectByRoot(context.Context, string) (*domain.Project, error) {
	return p.project, nil
}

type changeServiceSnapshot struct {
	snapshot domain.ChangeSourceSnapshot
	err      error
	calls    int
}

func (s *changeServiceSnapshot) Snapshot(context.Context, string) (domain.ChangeSourceSnapshot, error) {
	s.calls++
	return s.snapshot, s.err
}

type changeServiceArtifacts struct {
	identity  domain.ArtifactIdentity
	putCalls  int
	readCalls int
}

func (a *changeServiceArtifacts) Put(context.Context, []byte) (domain.ArtifactIdentity, error) {
	a.putCalls++
	return a.identity, nil
}

func (a *changeServiceArtifacts) Read(context.Context, domain.ArtifactIdentity) ([]byte, error) {
	a.readCalls++
	return nil, nil
}

type changeServiceState struct {
	change      domain.Change
	record      work.ChangeCreateRecord
	createCalls int
	replay      bool
}

func (s *changeServiceState) CreateChange(_ context.Context, record work.ChangeCreateRecord) (domain.Change, error) {
	s.createCalls++
	s.record = record
	return s.change, nil
}

func (s *changeServiceState) ReplayCreate(context.Context, string, domain.ProjectID, string, string) (domain.Change, bool, error) {
	return s.change, s.replay, nil
}

func (s *changeServiceState) FindChange(context.Context, domain.ChangeID) (domain.Change, error) {
	return domain.Change{}, domain.ErrChangeNotFound
}

func (s *changeServiceState) ListChanges(context.Context, string) ([]domain.Change, error) {
	return []domain.Change{}, nil
}

func (s *changeServiceState) ApplyCommand(context.Context, domain.ChangeID, string, domain.ChangeVersion, string, string) (domain.Change, error) {
	return domain.Change{}, nil
}

func (s *changeServiceState) ApplyDecision(context.Context, domain.ChangeID, string, domain.ChangeVersion, string, string, string) (domain.Change, error) {
	return domain.Change{}, nil
}

func (s *changeServiceState) ListChangeEvents(context.Context, domain.ChangeID) ([]domain.ChangeEvent, error) {
	return []domain.ChangeEvent{}, nil
}

func (s *changeServiceState) ListAgentRuns(context.Context, domain.ChangeID) ([]domain.AgentRun, error) {
	return []domain.AgentRun{}, nil
}

func (s *changeServiceState) ListArtifactRefs(context.Context, domain.ChangeID) ([]domain.ArtifactRef, error) {
	return []domain.ArtifactRef{}, nil
}

func (s *changeServiceState) ListHumanDecisions(context.Context, domain.ChangeID) ([]domain.HumanDecision, error) {
	return []domain.HumanDecision{}, nil
}

func (s *changeServiceState) FindArtifact(context.Context, domain.ChangeID, domain.ArtifactRefID) (domain.Artifact, error) {
	return domain.Artifact{}, domain.ErrArtifactNotFound
}
