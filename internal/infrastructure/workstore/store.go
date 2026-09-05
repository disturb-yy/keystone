// Package workstore 提供 Project Bootstrap 的 SQLite 持久化适配器。
package workstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/disturb-yy/keystone/internal/infrastructure/id"
	"github.com/disturb-yy/keystone/internal/infrastructure/migration"
	"github.com/disturb-yy/keystone/internal/work"
	"github.com/disturb-yy/keystone/internal/work/domain"
)

const projectSchemaSQL = `
CREATE TABLE t_projects (
    project_id TEXT PRIMARY KEY NOT NULL,
    repository_root TEXT NOT NULL UNIQUE,
    manifest_path TEXT NOT NULL,
    created_at TEXT NOT NULL
);
CREATE TABLE t_project_events (
    event_id TEXT PRIMARY KEY NOT NULL,
    project_id TEXT NOT NULL REFERENCES t_projects(project_id),
    type TEXT NOT NULL,
    occurred_at TEXT NOT NULL,
    UNIQUE(project_id, type)
);
CREATE TABLE t_project_initialization_intents (
    intent_id TEXT PRIMARY KEY NOT NULL,
    project_id TEXT NOT NULL,
    repository_root TEXT NOT NULL,
    idempotency_key TEXT NOT NULL,
    status TEXT NOT NULL,
    failure_code TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
CREATE UNIQUE INDEX ux_project_pending_root
    ON t_project_initialization_intents(repository_root) WHERE status = 'pending';
CREATE TABLE t_project_initialization_receipts (
    idempotency_key TEXT PRIMARY KEY NOT NULL,
    repository_root TEXT NOT NULL,
    project_id TEXT NOT NULL,
    status TEXT NOT NULL,
    failure_code TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL
);`

// Migrations 返回 Work 领域拥有的业务 Migration。
func Migrations() []migration.Migration {
	return []migration.Migration{{Version: 2, Name: "create_project_bootstrap", SQL: projectSchemaSQL}}
}

// Store 是 Project Bootstrap 的 SQLite 状态端口实现。
type Store struct {
	db  *sql.DB
	now func() time.Time
}

// New 创建 SQLite adapter。
func New(db *sql.DB) (*Store, error) {
	if db == nil {
		return nil, errors.New("create work store: nil database")
	}
	return &Store{db: db, now: time.Now}, nil
}

// Reserve 以幂等键或活动 root claim 取得可恢复 intent。
func (s *Store) Reserve(ctx context.Context, root, key string, candidate domain.ProjectID) (work.Reservation, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return work.Reservation{}, fmt.Errorf("begin intent reservation: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()
	var reservation work.Reservation
	receipt, err := readReceipt(ctx, tx, key)
	if err != nil {
		return reservation, err
	}
	if receipt != nil {
		if receipt.RepositoryRoot != root {
			return reservation, fmt.Errorf("%w: repository root differs", domain.ErrIdempotencyConflict)
		}
		if receipt.Status != "succeeded" {
			return reservation, errorForCode(receipt.FailureCode)
		}
		project, err := readProject(ctx, tx, receipt.ProjectID)
		if err != nil {
			return reservation, err
		}
		reservation.Receipt, reservation.Project = receipt, project
		if err := tx.Commit(); err != nil {
			return work.Reservation{}, fmt.Errorf("commit receipt reservation: %w", err)
		}
		committed = true
		return reservation, nil
	}
	keyIntent, err := readIntentByKey(ctx, tx, key)
	if err != nil {
		return reservation, err
	}
	if keyIntent != nil {
		if keyIntent.RepositoryRoot != root {
			return reservation, fmt.Errorf("%w: repository root differs", domain.ErrIdempotencyConflict)
		}
		if keyIntent.Status != domain.IntentPending {
			return reservation, errorForCode(keyIntent.FailureCode)
		}
		reservation.Intent = *keyIntent
		if err := tx.Commit(); err != nil {
			return work.Reservation{}, fmt.Errorf("commit intent reservation: %w", err)
		}
		committed = true
		return reservation, nil
	}
	intent, err := readPendingIntent(ctx, tx, root)
	if err != nil {
		return reservation, err
	}
	if intent == nil {
		project, projectErr := readProjectByRoot(ctx, tx, root)
		if projectErr != nil && !errors.Is(projectErr, domain.ErrProjectNotFound) {
			return reservation, projectErr
		}
		projectID := candidate
		if projectErr == nil {
			projectID = project.Identity.ProjectID
		}
		intent = &domain.ProjectInitializationIntent{ID: id.New(), ProjectID: projectID, RepositoryRoot: root, IdempotencyKey: key, Status: domain.IntentPending}
		stamp := s.now().UTC().Format(time.RFC3339Nano)
		const query = `INSERT INTO t_project_initialization_intents (intent_id, project_id, repository_root, idempotency_key, status, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`
		if _, err := tx.ExecContext(ctx, query, intent.ID, intent.ProjectID, intent.RepositoryRoot, intent.IdempotencyKey, intent.Status, stamp, stamp); err != nil {
			return reservation, fmt.Errorf("insert project initialization intent: %w", err)
		}
	}
	reservation.Intent = *intent
	if err := tx.Commit(); err != nil {
		return work.Reservation{}, fmt.Errorf("commit intent reservation: %w", err)
	}
	committed = true
	return reservation, nil
}

// AdoptIntentProjectID 将已核验 Manifest 的身份写入 pending intent。
func (s *Store) AdoptIntentProjectID(ctx context.Context, intentID string, projectID domain.ProjectID) error {
	result, err := s.db.ExecContext(ctx, `UPDATE t_project_initialization_intents SET project_id = ?, updated_at = ? WHERE intent_id = ? AND status = 'pending'`, projectID, s.now().UTC().Format(time.RFC3339Nano), intentID)
	if err != nil {
		return fmt.Errorf("adopt intent project id: %w", err)
	}
	if count, err := result.RowsAffected(); err != nil {
		return fmt.Errorf("check adopted intent: %w", err)
	} else if count != 1 {
		return fmt.Errorf("adopt intent project id: %w", domain.ErrInternal)
	}
	return nil
}

// FailIntent 持久化确定性失败并释放 pending root claim。
func (s *Store) FailIntent(ctx context.Context, intent domain.ProjectInitializationIntent, key, code string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin failed project initialization: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()
	current, err := readIntentByID(ctx, tx, intent.ID)
	if err != nil {
		return err
	}
	if current == nil || current.Status != domain.IntentPending {
		return fmt.Errorf("fail project initialization intent: %w", domain.ErrInternal)
	}
	keys := []string{current.IdempotencyKey}
	if key != current.IdempotencyKey {
		keys = append(keys, key)
	}
	stamp := s.now().UTC().Format(time.RFC3339Nano)
	for _, receiptKey := range keys {
		if err := s.writeFailureReceipt(ctx, tx, receiptKey, current.RepositoryRoot, current.ProjectID, code, stamp); err != nil {
			return err
		}
	}
	result, err := tx.ExecContext(ctx, `UPDATE t_project_initialization_intents SET status = 'failed', failure_code = ?, updated_at = ? WHERE intent_id = ? AND status = 'pending'`, code, stamp, intent.ID)
	if err != nil {
		return fmt.Errorf("fail project initialization intent: %w", err)
	}
	if count, err := result.RowsAffected(); err != nil {
		return fmt.Errorf("check failed project initialization intent: %w", err)
	} else if count != 1 {
		return fmt.Errorf("fail project initialization intent: %w", domain.ErrInternal)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit failed project initialization: %w", err)
	}
	committed = true
	return nil
}

// FindProject 查询单一 Project。
func (s *Store) FindProject(ctx context.Context, projectID domain.ProjectID) (*domain.Project, error) {
	return readProject(ctx, s.db, projectID)
}

// Finalize 在一个事务中完成 Project、首个 Event 和 Receipt。
func (s *Store) Finalize(ctx context.Context, key string, intent domain.ProjectInitializationIntent, manifest domain.ProjectManifest, binding domain.RepositoryBinding, rebindFrom string) (project domain.Project, err error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return project, fmt.Errorf("begin project finalization: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			err = errors.Join(err, tx.Rollback())
		}
	}()
	projectPtr, readErr := readProject(ctx, tx, manifest.ProjectID)
	if readErr != nil && !errors.Is(readErr, domain.ErrProjectNotFound) {
		return project, readErr
	}
	if projectPtr == nil {
		rootProject, rootErr := readProjectByRoot(ctx, tx, binding.Root)
		if rootErr == nil && rootProject.Identity.ProjectID != manifest.ProjectID {
			return project, fmt.Errorf("%w: repository root has another project", domain.ErrProjectIdentityConflict)
		}
		if rootErr != nil && !errors.Is(rootErr, domain.ErrProjectNotFound) {
			return project, rootErr
		}
		created := s.now().UTC()
		project = domain.Project{Identity: domain.RepositoryIdentity{ProjectID: manifest.ProjectID}, Binding: binding, CreatedAt: created}
		if _, err := tx.ExecContext(ctx, `INSERT INTO t_projects (project_id, repository_root, manifest_path, created_at) VALUES (?, ?, ?, ?)`, project.Identity.ProjectID, binding.Root, binding.ManifestPath, created.Format(time.RFC3339Nano)); err != nil {
			return project, fmt.Errorf("insert project: %w", err)
		}
		event := domain.ProjectInitialized{EventID: id.New(), ProjectID: manifest.ProjectID, OccurredAt: created}
		if _, err := tx.ExecContext(ctx, `INSERT INTO t_project_events (event_id, project_id, type, occurred_at) VALUES (?, ?, ?, ?)`, event.EventID, event.ProjectID, domain.ProjectInitializedType, event.OccurredAt.Format(time.RFC3339Nano)); err != nil {
			return project, fmt.Errorf("insert project initialized event: %w", err)
		}
	} else {
		if err := validateProjectEvent(ctx, tx, manifest.ProjectID); err != nil {
			return project, err
		}
		project = *projectPtr
		if project.Binding.Root != binding.Root {
			if rebindFrom == "" || project.Binding.Root != rebindFrom {
				return project, fmt.Errorf("%w: active repository root differs", domain.ErrProjectIdentityConflict)
			}
			result, err := tx.ExecContext(ctx, `UPDATE t_projects SET repository_root = ?, manifest_path = ? WHERE project_id = ? AND repository_root = ?`, binding.Root, binding.ManifestPath, manifest.ProjectID, rebindFrom)
			if err != nil {
				return project, fmt.Errorf("rebind project: %w", err)
			}
			if count, err := result.RowsAffected(); err != nil {
				return project, fmt.Errorf("check rebind project: %w", err)
			} else if count != 1 {
				return project, fmt.Errorf("%w: repository binding changed during rebind", domain.ErrProjectIdentityConflict)
			}
			project.Binding = binding
		}
	}
	receiptKeys := []string{intent.IdempotencyKey}
	if key != intent.IdempotencyKey {
		receiptKeys = append(receiptKeys, key)
	}
	stamp := s.now().UTC().Format(time.RFC3339Nano)
	for _, receiptKey := range receiptKeys {
		if err := s.writeSuccessReceipt(ctx, tx, receiptKey, binding.Root, manifest.ProjectID, stamp); err != nil {
			return project, err
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE t_project_initialization_intents SET status = 'succeeded', updated_at = ? WHERE intent_id = ? AND status = 'pending'`, s.now().UTC().Format(time.RFC3339Nano), intent.ID); err != nil {
		return project, fmt.Errorf("complete project initialization intent: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return project, fmt.Errorf("commit project finalization: %w", err)
	}
	committed = true
	return project, nil
}

func (s *Store) writeSuccessReceipt(ctx context.Context, tx *sql.Tx, key, root string, projectID domain.ProjectID, stamp string) error {
	existing, err := readReceipt(ctx, tx, key)
	if err != nil {
		return err
	}
	if existing != nil {
		if existing.RepositoryRoot != root {
			return fmt.Errorf("%w: receipt key differs", domain.ErrIdempotencyConflict)
		}
		if existing.Status != "succeeded" {
			return errorForCode(existing.FailureCode)
		}
		if existing.ProjectID != projectID {
			return fmt.Errorf("%w: receipt project differs", domain.ErrProjectIdentityConflict)
		}
		return nil
	}
	const receiptQuery = `INSERT INTO t_project_initialization_receipts (idempotency_key, repository_root, project_id, status, created_at) VALUES (?, ?, ?, 'succeeded', ?)`
	if _, err := tx.ExecContext(ctx, receiptQuery, key, root, projectID, stamp); err != nil {
		return fmt.Errorf("insert project initialization receipt: %w", err)
	}
	return nil
}

func (s *Store) writeFailureReceipt(ctx context.Context, tx *sql.Tx, key, root string, projectID domain.ProjectID, code, stamp string) error {
	existing, err := readReceipt(ctx, tx, key)
	if err != nil {
		return err
	}
	if existing != nil {
		if existing.RepositoryRoot != root {
			return fmt.Errorf("%w: receipt key differs", domain.ErrIdempotencyConflict)
		}
		if existing.Status == "failed" && existing.FailureCode == code {
			return nil
		}
		return fmt.Errorf("%w: receipt already finalized", domain.ErrInternal)
	}
	const query = `INSERT INTO t_project_initialization_receipts (idempotency_key, repository_root, project_id, status, failure_code, created_at) VALUES (?, ?, ?, 'failed', ?, ?)`
	if _, err := tx.ExecContext(ctx, query, key, root, projectID, code, stamp); err != nil {
		return fmt.Errorf("insert failed project initialization receipt: %w", err)
	}
	return nil
}

// ListEvents 返回按发生时间和 event_id 升序排列的事件。
func (s *Store) ListEvents(ctx context.Context, projectID domain.ProjectID) ([]domain.ProjectInitialized, error) {
	if _, err := readProject(ctx, s.db, projectID); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT event_id, project_id, type, occurred_at FROM t_project_events WHERE project_id = ? ORDER BY occurred_at, event_id`, projectID)
	if err != nil {
		return nil, fmt.Errorf("query project events: %w", err)
	}
	defer rows.Close()
	var events []domain.ProjectInitialized
	for rows.Next() {
		var eventID, eventProjectID, eventType, occurred string
		if err := rows.Scan(&eventID, &eventProjectID, &eventType, &occurred); err != nil {
			return nil, err
		}
		if eventType != domain.ProjectInitializedType {
			return nil, fmt.Errorf("%w: unknown project event type", domain.ErrInternal)
		}
		timeValue, err := time.Parse(time.RFC3339Nano, occurred)
		if err != nil {
			return nil, fmt.Errorf("%w: invalid event time", domain.ErrInternal)
		}
		events = append(events, domain.ProjectInitialized{EventID: eventID, ProjectID: domain.ProjectID(eventProjectID), OccurredAt: timeValue.UTC()})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read project events: %w", err)
	}
	return events, nil
}

func readProject(ctx context.Context, queryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, projectID domain.ProjectID) (*domain.Project, error) {
	var root, manifestPath, created string
	err := queryer.QueryRowContext(ctx, `SELECT repository_root, manifest_path, created_at FROM t_projects WHERE project_id = ?`, projectID).Scan(&root, &manifestPath, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrProjectNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("read project: %w", err)
	}
	createdAt, err := time.Parse(time.RFC3339Nano, created)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid project time", domain.ErrInternal)
	}
	return &domain.Project{Identity: domain.RepositoryIdentity{ProjectID: projectID}, Binding: domain.RepositoryBinding{Root: root, ManifestPath: manifestPath}, CreatedAt: createdAt.UTC()}, nil
}

func readProjectByRoot(ctx context.Context, queryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, root string) (*domain.Project, error) {
	var projectID domain.ProjectID
	err := queryer.QueryRowContext(ctx, `SELECT project_id FROM t_projects WHERE repository_root = ?`, root).Scan(&projectID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrProjectNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("read project by root: %w", err)
	}
	return readProject(ctx, queryer, projectID)
}

func readPendingIntent(ctx context.Context, tx *sql.Tx, root string) (*domain.ProjectInitializationIntent, error) {
	var intent domain.ProjectInitializationIntent
	err := tx.QueryRowContext(ctx, `SELECT intent_id, project_id, repository_root, idempotency_key, status, failure_code FROM t_project_initialization_intents WHERE repository_root = ? AND status = 'pending'`, root).Scan(&intent.ID, &intent.ProjectID, &intent.RepositoryRoot, &intent.IdempotencyKey, &intent.Status, &intent.FailureCode)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read pending intent: %w", err)
	}
	return &intent, nil
}

func readIntentByKey(ctx context.Context, tx *sql.Tx, key string) (*domain.ProjectInitializationIntent, error) {
	var intent domain.ProjectInitializationIntent
	err := tx.QueryRowContext(ctx, `SELECT intent_id, project_id, repository_root, idempotency_key, status, failure_code FROM t_project_initialization_intents WHERE idempotency_key = ? ORDER BY created_at DESC LIMIT 1`, key).Scan(&intent.ID, &intent.ProjectID, &intent.RepositoryRoot, &intent.IdempotencyKey, &intent.Status, &intent.FailureCode)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read intent by key: %w", err)
	}
	return &intent, nil
}

func readIntentByID(ctx context.Context, tx *sql.Tx, intentID string) (*domain.ProjectInitializationIntent, error) {
	var intent domain.ProjectInitializationIntent
	err := tx.QueryRowContext(ctx, `SELECT intent_id, project_id, repository_root, idempotency_key, status, failure_code FROM t_project_initialization_intents WHERE intent_id = ?`, intentID).Scan(&intent.ID, &intent.ProjectID, &intent.RepositoryRoot, &intent.IdempotencyKey, &intent.Status, &intent.FailureCode)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read intent by id: %w", err)
	}
	return &intent, nil
}

func readReceipt(ctx context.Context, queryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, key string) (*domain.ProjectInitializationReceipt, error) {
	var receipt domain.ProjectInitializationReceipt
	err := queryer.QueryRowContext(ctx, `SELECT idempotency_key, repository_root, project_id, status, failure_code FROM t_project_initialization_receipts WHERE idempotency_key = ?`, key).Scan(&receipt.IdempotencyKey, &receipt.RepositoryRoot, &receipt.ProjectID, &receipt.Status, &receipt.FailureCode)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read initialization receipt: %w", err)
	}
	return &receipt, nil
}

func validateProjectEvent(ctx context.Context, tx *sql.Tx, projectID domain.ProjectID) error {
	rows, err := tx.QueryContext(ctx, `SELECT project_id, type FROM t_project_events WHERE project_id = ?`, projectID)
	if err != nil {
		return fmt.Errorf("query project initialized event: %w", err)
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		var eventProjectID, eventType string
		if err := rows.Scan(&eventProjectID, &eventType); err != nil {
			return err
		}
		count++
		if eventProjectID != string(projectID) || eventType != domain.ProjectInitializedType {
			return fmt.Errorf("%w: project initialized event mismatch", domain.ErrInternal)
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if count != 1 {
		return fmt.Errorf("%w: project initialized event count is %d", domain.ErrInternal, count)
	}
	return nil
}

func errorForCode(code string) error {
	switch code {
	case "manifest_invalid":
		return domain.ErrManifestInvalid
	case "project_identity_conflict":
		return domain.ErrProjectIdentityConflict
	case "idempotency_conflict":
		return domain.ErrIdempotencyConflict
	default:
		return domain.ErrInternal
	}
}
