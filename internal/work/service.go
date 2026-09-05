// Package work 提供 Project Bootstrap 的 Application 用例。
package work

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/disturb-yy/keystone/internal/work/domain"
)

// RepositoryPort 解析调用路径对应的物理 Git RepositoryBinding。
type RepositoryPort interface {
	Discover(context.Context, string) (domain.RepositoryBinding, error)
	RootExists(context.Context, string) (bool, error)
}

// ManifestPort 严格创建或读取 ProjectManifest。
type ManifestPort interface {
	Ensure(context.Context, domain.RepositoryBinding, domain.ProjectID) (domain.ProjectManifest, error)
}

// Reservation 是 SQLite 中的活动 intent 或已完成 receipt。
type Reservation struct {
	Intent  domain.ProjectInitializationIntent
	Receipt *domain.ProjectInitializationReceipt
	Project *domain.Project
}

// StatePort 持久化 intent、receipt、Project 和 ProjectInitialized。
type StatePort interface {
	Reserve(context.Context, string, string, domain.ProjectID) (Reservation, error)
	AdoptIntentProjectID(context.Context, string, domain.ProjectID) error
	FailIntent(context.Context, domain.ProjectInitializationIntent, string, string) error
	FindProject(context.Context, domain.ProjectID) (*domain.Project, error)
	Finalize(context.Context, string, domain.ProjectInitializationIntent, domain.ProjectManifest, domain.RepositoryBinding, string) (domain.Project, error)
	ListEvents(context.Context, domain.ProjectID) ([]domain.ProjectInitialized, error)
}

// InitializeRequest 是 Application 层的初始化输入。
type InitializeRequest struct {
	RepositoryPath string
	IdempotencyKey string
}

// Service 编排 Repository、Manifest 和本机权威状态。
type Service struct {
	repository RepositoryPort
	manifest   ManifestPort
	state      StatePort
	now        func() time.Time
}

// NewService 创建 Project Bootstrap Application Service。
func NewService(repository RepositoryPort, manifest ManifestPort, state StatePort) (*Service, error) {
	if repository == nil || manifest == nil || state == nil {
		return nil, errors.New("create project service: all ports are required")
	}
	return &Service{repository: repository, manifest: manifest, state: state, now: time.Now}, nil
}

// Initialize 将 RepositoryBinding 协调为当前 LocalStateRoot 的 Project。
func (s *Service) Initialize(ctx context.Context, request InitializeRequest) (domain.Project, error) {
	if ctx == nil {
		return domain.Project{}, fmt.Errorf("initialize project: %w", domain.ErrInvalidRequest)
	}
	if request.IdempotencyKey == "" {
		return domain.Project{}, fmt.Errorf("initialize project: %w", domain.ErrInvalidRequest)
	}
	binding, err := s.repository.Discover(ctx, request.RepositoryPath)
	if err != nil {
		return domain.Project{}, fmt.Errorf("discover repository: %w", err)
	}
	reservation, err := s.state.Reserve(ctx, binding.Root, request.IdempotencyKey, domain.NewProjectID())
	if err != nil {
		return domain.Project{}, fmt.Errorf("reserve project initialization: %w", err)
	}
	if reservation.Receipt != nil && reservation.Project != nil {
		return *reservation.Project, nil
	}
	manifest, err := s.manifest.Ensure(ctx, binding, reservation.Intent.ProjectID)
	if err != nil {
		return domain.Project{}, s.handleManifestFailure(ctx, reservation.Intent, request.IdempotencyKey, err)
	}
	if manifest.ProjectID != reservation.Intent.ProjectID {
		if err := s.state.AdoptIntentProjectID(ctx, reservation.Intent.ID, manifest.ProjectID); err != nil {
			return domain.Project{}, fmt.Errorf("adopt manifest project identity: %w", err)
		}
		reservation.Intent.ProjectID = manifest.ProjectID
	}
	rebindFrom, err := s.allowRebind(ctx, manifest.ProjectID, binding.Root)
	if err != nil {
		return domain.Project{}, s.failDeterministic(ctx, reservation.Intent, request.IdempotencyKey, err)
	}
	project, err := s.state.Finalize(ctx, request.IdempotencyKey, reservation.Intent, manifest, binding, rebindFrom)
	if err != nil {
		return domain.Project{}, s.failDeterministic(ctx, reservation.Intent, request.IdempotencyKey, err)
	}
	return project, nil
}

func (s *Service) handleManifestFailure(ctx context.Context, intent domain.ProjectInitializationIntent, key string, err error) error {
	if errors.Is(err, domain.ErrManifestInvalid) {
		return s.failDeterministic(ctx, intent, key, err)
	}
	return fmt.Errorf("ensure project manifest: %w", err)
}

func (s *Service) failDeterministic(ctx context.Context, intent domain.ProjectInitializationIntent, key string, err error) error {
	if !isDeterministicFailure(err) {
		return err
	}
	if markErr := s.state.FailIntent(ctx, intent, key, codeFor(err)); markErr != nil {
		return errors.Join(err, fmt.Errorf("record project initialization failure: %w", markErr))
	}
	return err
}

func (s *Service) allowRebind(ctx context.Context, projectID domain.ProjectID, root string) (string, error) {
	project, err := s.state.FindProject(ctx, projectID)
	if errors.Is(err, domain.ErrProjectNotFound) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if project.Binding.Root == root {
		return "", nil
	}
	exists, err := s.repository.RootExists(ctx, project.Binding.Root)
	if err != nil {
		return "", fmt.Errorf("verify previous repository binding: %w", err)
	}
	if exists {
		return "", fmt.Errorf("%w: another active repository root exists", domain.ErrProjectIdentityConflict)
	}
	return project.Binding.Root, nil
}

func isDeterministicFailure(err error) bool {
	return errors.Is(err, domain.ErrManifestInvalid) ||
		errors.Is(err, domain.ErrIdempotencyConflict) ||
		errors.Is(err, domain.ErrProjectIdentityConflict)
}

// GetProject 查询当前 LocalStateRoot 内的权威 Project。
func (s *Service) GetProject(ctx context.Context, projectID domain.ProjectID) (domain.Project, error) {
	if err := projectID.Validate(); err != nil {
		return domain.Project{}, fmt.Errorf("get project: %w", domain.ErrProjectNotFound)
	}
	project, err := s.state.FindProject(ctx, projectID)
	if err != nil {
		return domain.Project{}, fmt.Errorf("get project: %w", err)
	}
	return *project, nil
}

// ListEvents 查询并返回稳定排序的 ProjectInitialized 事实。
func (s *Service) ListEvents(ctx context.Context, projectID domain.ProjectID) ([]domain.ProjectInitialized, error) {
	if err := projectID.Validate(); err != nil {
		return nil, fmt.Errorf("list project events: %w", domain.ErrProjectNotFound)
	}
	return s.state.ListEvents(ctx, projectID)
}

// ErrorCode 返回 HTTP 边界可使用的稳定错误代码。
func ErrorCode(err error) string {
	if errors.Is(err, domain.ErrInvalidRequest) {
		return "invalid_request"
	}
	if errors.Is(err, domain.ErrRepositoryUnsupported) {
		return "repository_unsupported"
	}
	if errors.Is(err, domain.ErrManifestInvalid) {
		return "manifest_invalid"
	}
	if errors.Is(err, domain.ErrManifestUnavailable) {
		return "manifest_unavailable"
	}
	if errors.Is(err, domain.ErrIdempotencyConflict) {
		return "idempotency_conflict"
	}
	if errors.Is(err, domain.ErrProjectIdentityConflict) {
		return "project_identity_conflict"
	}
	if errors.Is(err, domain.ErrProjectNotFound) {
		return "project_not_found"
	}
	return "internal_error"
}

func codeFor(err error) string { return ErrorCode(err) }
