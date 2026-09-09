package workstore

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/disturb-yy/keystone/internal/work/domain"
)

const (
	// DefaultDashboardPageLimit 是 Dashboard 列表 Query 的默认条数。
	DefaultDashboardPageLimit = 50
	// MaxDashboardPageLimit 是 Dashboard 列表 Query 允许的最大条数。
	MaxDashboardPageLimit        = 200
	maxDashboardObservationItems = 100
)

// DashboardPage 是 Dashboard Query 的已校验分页输入。
type DashboardPage struct {
	Limit  int
	Cursor string
}

// ProjectSummary 是 Projects inventory 的只读投影。
type ProjectSummary struct {
	ProjectID         domain.ProjectID
	RepositoryRoot    string
	CreatedAt         time.Time
	ChangeCount       int
	ActiveChangeCount int
}

// ProjectPage 是 Projects inventory 的分页结果。
type ProjectPage struct {
	Projects   []ProjectSummary
	HasMore    bool
	NextCursor string
}

// ProjectChangesPage 是 Project-scoped Change list 的分页结果。
type ProjectChangesPage struct {
	ProjectID  domain.ProjectID
	Changes    []domain.Change
	HasMore    bool
	NextCursor string
}

// NeedsHumanRecord 是 Daemon 已持久化 human_required 状态的安全投影。
type NeedsHumanRecord struct {
	Change               domain.Change
	RequiredAt           time.Time
	ReasonCode           string
	ReasonSummary        string
	EvidenceArtifactRefs []domain.ArtifactRefID
}

// NeedsHumanPage 是 Needs Human inventory 的分页结果。
type NeedsHumanPage struct {
	Items      []NeedsHumanRecord
	HasMore    bool
	NextCursor string
}

// ArtifactObservation 是不含 Artifact 原文的安全摘要。
type ArtifactObservation struct {
	Ref        domain.ArtifactRef
	ByteLength int64
	MediaType  string
}

// WorkerHealth 是已注册 Worker 的状态、能力和原始心跳时间摘要。
type WorkerHealth struct {
	WorkerID        string
	ProtocolVersion string
	Status          string
	Capabilities    []string
	LastHeartbeatAt *time.Time
}

type dashboardCursor struct {
	Kind       string `json:"kind"`
	ProjectID  string `json:"project_id,omitempty"`
	CreatedAt  string `json:"created_at,omitempty"`
	RequiredAt string `json:"required_at,omitempty"`
	ChangeID   string `json:"change_id,omitempty"`
}

func normalizeDashboardPage(page DashboardPage) (DashboardPage, error) {
	if page.Limit == 0 {
		page.Limit = DefaultDashboardPageLimit
	}
	if page.Limit < 1 || page.Limit > MaxDashboardPageLimit {
		return DashboardPage{}, fmt.Errorf("dashboard page limit %d is outside 1..%d: %w", page.Limit, MaxDashboardPageLimit, domain.ErrInvalidRequest)
	}
	return page, nil
}

func encodeDashboardCursor(cursor dashboardCursor) (string, error) {
	encoded, err := json.Marshal(cursor)
	if err != nil {
		return "", fmt.Errorf("encode dashboard cursor: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(encoded), nil
}

func decodeDashboardCursor(value, kind string) (dashboardCursor, error) {
	if strings.TrimSpace(value) == "" {
		return dashboardCursor{}, nil
	}
	encoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return dashboardCursor{}, fmt.Errorf("decode dashboard cursor: %w", domain.ErrInvalidRequest)
	}
	var cursor dashboardCursor
	if err := json.Unmarshal(encoded, &cursor); err != nil || cursor.Kind != kind {
		return dashboardCursor{}, fmt.Errorf("dashboard cursor kind is invalid: %w", domain.ErrInvalidRequest)
	}
	if (kind == "projects" && cursor.ProjectID == "") || (kind == "changes" && (cursor.CreatedAt == "" || cursor.ProjectID == "" || cursor.ChangeID == "")) || (kind == "needs_human" && (cursor.RequiredAt == "" || cursor.ChangeID == "")) {
		return dashboardCursor{}, fmt.Errorf("dashboard cursor fields are invalid: %w", domain.ErrInvalidRequest)
	}
	return cursor, nil
}

// ListProjectSummaries 返回按 ProjectID 升序排列的有界 inventory。
func (s *Store) ListProjectSummaries(ctx context.Context, page DashboardPage) (ProjectPage, error) {
	page, err := normalizeDashboardPage(page)
	if err != nil {
		return ProjectPage{}, err
	}
	cursor, err := decodeDashboardCursor(page.Cursor, "projects")
	if err != nil {
		return ProjectPage{}, err
	}
	query := `SELECT p.project_id, p.repository_root, p.created_at,
COALESCE((SELECT COUNT(1) FROM t_changes c WHERE c.project_id = p.project_id), 0),
COALESCE((SELECT COUNT(1) FROM t_changes c WHERE c.project_id = p.project_id AND c.status = 'active'), 0)
FROM t_projects p`
	args := make([]any, 0, 2)
	if cursor.ProjectID != "" {
		query += ` WHERE p.project_id > ?`
		args = append(args, cursor.ProjectID)
	}
	query += ` ORDER BY p.project_id ASC LIMIT ?`
	args = append(args, page.Limit+1)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return ProjectPage{}, fmt.Errorf("list project summaries: %w", err)
	}
	defer rows.Close()
	projects := make([]ProjectSummary, 0, page.Limit)
	for rows.Next() {
		var project ProjectSummary
		var created string
		if err := rows.Scan(&project.ProjectID, &project.RepositoryRoot, &created, &project.ChangeCount, &project.ActiveChangeCount); err != nil {
			return ProjectPage{}, fmt.Errorf("scan project summary: %w", err)
		}
		project.CreatedAt, err = parseStamp(created)
		if err != nil {
			return ProjectPage{}, err
		}
		projects = append(projects, project)
	}
	if err := rows.Err(); err != nil {
		return ProjectPage{}, fmt.Errorf("read project summaries: %w", err)
	}
	return projectPage(projects, page.Limit, func(project ProjectSummary) (dashboardCursor, error) {
		return dashboardCursor{Kind: "projects", ProjectID: string(project.ProjectID)}, nil
	})
}

func projectPage(projects []ProjectSummary, limit int, cursor func(ProjectSummary) (dashboardCursor, error)) (ProjectPage, error) {
	page := ProjectPage{Projects: projects}
	if len(projects) <= limit {
		return page, nil
	}
	page.HasMore = true
	page.Projects = projects[:limit]
	value, err := cursor(page.Projects[len(page.Projects)-1])
	if err != nil {
		return ProjectPage{}, err
	}
	page.NextCursor, err = encodeDashboardCursor(value)
	if err != nil {
		return ProjectPage{}, err
	}
	return page, nil
}

// FindProjectSummary 返回一个 Project 的 Dashboard 摘要。
func (s *Store) FindProjectSummary(ctx context.Context, projectID domain.ProjectID) (ProjectSummary, error) {
	var project ProjectSummary
	var created string
	err := s.db.QueryRowContext(ctx, `SELECT p.project_id, p.repository_root, p.created_at,
COALESCE((SELECT COUNT(1) FROM t_changes c WHERE c.project_id = p.project_id), 0),
COALESCE((SELECT COUNT(1) FROM t_changes c WHERE c.project_id = p.project_id AND c.status = 'active'), 0)
FROM t_projects p WHERE p.project_id = ?`, projectID).Scan(&project.ProjectID, &project.RepositoryRoot, &created, &project.ChangeCount, &project.ActiveChangeCount)
	if errors.Is(err, sql.ErrNoRows) {
		return ProjectSummary{}, domain.ErrProjectNotFound
	}
	if err != nil {
		return ProjectSummary{}, fmt.Errorf("find project summary: %w", err)
	}
	project.CreatedAt, err = parseStamp(created)
	if err != nil {
		return ProjectSummary{}, err
	}
	return project, nil
}

// ListProjectChanges 返回按 created_at、change_id 倒序排列的 Project Change 摘要。
func (s *Store) ListProjectChanges(ctx context.Context, projectID domain.ProjectID, page DashboardPage) (ProjectChangesPage, error) {
	if _, err := s.FindProjectSummary(ctx, projectID); err != nil {
		return ProjectChangesPage{}, err
	}
	page, err := normalizeDashboardPage(page)
	if err != nil {
		return ProjectChangesPage{}, err
	}
	cursor, err := decodeDashboardCursor(page.Cursor, "changes")
	if err != nil {
		return ProjectChangesPage{}, err
	}
	if cursor.ProjectID != "" && cursor.ProjectID != string(projectID) {
		return ProjectChangesPage{}, fmt.Errorf("dashboard change cursor project differs: %w", domain.ErrInvalidRequest)
	}
	args := []any{projectID}
	query := `SELECT change_id FROM t_changes WHERE project_id = ?`
	if cursor.CreatedAt != "" {
		query += ` AND (created_at < ? OR (created_at = ? AND change_id < ?))`
		args = append(args, cursor.CreatedAt, cursor.CreatedAt, cursor.ChangeID)
	}
	query += ` ORDER BY created_at DESC, change_id DESC LIMIT ?`
	args = append(args, page.Limit+1)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return ProjectChangesPage{}, fmt.Errorf("list project changes: %w", err)
	}
	changeIDs := make([]domain.ChangeID, 0, page.Limit)
	for rows.Next() {
		var changeID domain.ChangeID
		if err := rows.Scan(&changeID); err != nil {
			return ProjectChangesPage{}, fmt.Errorf("scan project change id: %w", err)
		}
		changeIDs = append(changeIDs, changeID)
	}
	if err := rows.Err(); err != nil {
		return ProjectChangesPage{}, fmt.Errorf("read project changes: %w", err)
	}
	if err := rows.Close(); err != nil {
		return ProjectChangesPage{}, fmt.Errorf("close project changes: %w", err)
	}
	changes := make([]domain.Change, 0, len(changeIDs))
	for _, changeID := range changeIDs {
		change, err := readChange(ctx, s.db, changeID)
		if err != nil {
			return ProjectChangesPage{}, err
		}
		changes = append(changes, change)
	}
	result := ProjectChangesPage{ProjectID: projectID, Changes: changes}
	if len(changes) <= page.Limit {
		return result, nil
	}
	result.HasMore = true
	result.Changes = changes[:page.Limit]
	last := result.Changes[len(result.Changes)-1]
	result.NextCursor, err = encodeDashboardCursor(dashboardCursor{Kind: "changes", ProjectID: string(projectID), CreatedAt: stamp(last.CreatedAt), ChangeID: string(last.ID)})
	if err != nil {
		return ProjectChangesPage{}, err
	}
	return result, nil
}

// ListNeedsHuman 返回 Daemon 已标记的 human_required Change inventory。
func (s *Store) ListNeedsHuman(ctx context.Context, projectID domain.ProjectID, changeID domain.ChangeID, page DashboardPage) (NeedsHumanPage, error) {
	page, err := normalizeDashboardPage(page)
	if err != nil {
		return NeedsHumanPage{}, err
	}
	cursor, err := decodeDashboardCursor(page.Cursor, "needs_human")
	if err != nil {
		return NeedsHumanPage{}, err
	}
	query := `SELECT c.change_id, c.project_id,
COALESCE((SELECT e.occurred_at FROM t_project_events e WHERE e.change_id = c.change_id AND e.type = 'ChangeHumanRequired' ORDER BY e.event_sequence DESC LIMIT 1), c.updated_at)
FROM t_changes c WHERE c.status = 'human_required'`
	args := make([]any, 0, 8)
	if projectID != "" {
		query += ` AND c.project_id = ?`
		args = append(args, projectID)
	}
	if changeID != "" {
		query += ` AND c.change_id = ?`
		args = append(args, changeID)
	}
	if cursor.RequiredAt != "" {
		query += ` AND (COALESCE((SELECT e.occurred_at FROM t_project_events e WHERE e.change_id = c.change_id AND e.type = 'ChangeHumanRequired' ORDER BY e.event_sequence DESC LIMIT 1), c.updated_at) > ? OR (COALESCE((SELECT e.occurred_at FROM t_project_events e WHERE e.change_id = c.change_id AND e.type = 'ChangeHumanRequired' ORDER BY e.event_sequence DESC LIMIT 1), c.updated_at) = ? AND c.change_id > ?))`
		args = append(args, cursor.RequiredAt, cursor.RequiredAt, cursor.ChangeID)
	}
	query += ` ORDER BY COALESCE((SELECT e.occurred_at FROM t_project_events e WHERE e.change_id = c.change_id AND e.type = 'ChangeHumanRequired' ORDER BY e.event_sequence DESC LIMIT 1), c.updated_at) ASC, c.change_id ASC LIMIT ?`
	args = append(args, page.Limit+1)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return NeedsHumanPage{}, fmt.Errorf("list needs human: %w", err)
	}
	type needsHumanRow struct {
		changeID   domain.ChangeID
		projectID  domain.ProjectID
		requiredAt string
	}
	rowsToRead := make([]needsHumanRow, 0, page.Limit)
	for rows.Next() {
		var changeIDValue domain.ChangeID
		var projectIDValue domain.ProjectID
		var requiredAt string
		if err := rows.Scan(&changeIDValue, &projectIDValue, &requiredAt); err != nil {
			return NeedsHumanPage{}, fmt.Errorf("scan needs human: %w", err)
		}
		rowsToRead = append(rowsToRead, needsHumanRow{changeID: changeIDValue, projectID: projectIDValue, requiredAt: requiredAt})
	}
	if err := rows.Err(); err != nil {
		return NeedsHumanPage{}, fmt.Errorf("read needs human: %w", err)
	}
	if err := rows.Close(); err != nil {
		return NeedsHumanPage{}, fmt.Errorf("close needs human: %w", err)
	}
	items := make([]NeedsHumanRecord, 0, len(rowsToRead))
	for _, row := range rowsToRead {
		change, err := readChange(ctx, s.db, row.changeID)
		if err != nil {
			return NeedsHumanPage{}, err
		}
		required, err := parseStamp(row.requiredAt)
		if err != nil {
			return NeedsHumanPage{}, err
		}
		item := NeedsHumanRecord{Change: change, RequiredAt: required, ReasonCode: "change_human_required", ReasonSummary: "Daemon marked this Change as requiring a human decision."}
		var eventID string
		if err := s.db.QueryRowContext(ctx, `SELECT event_id FROM t_project_events WHERE change_id = ? AND type = 'ChangeHumanRequired' ORDER BY event_sequence DESC LIMIT 1`, row.changeID).Scan(&eventID); err == nil {
			item.EvidenceArtifactRefs, err = listEventArtifactRefs(ctx, s.db, eventID)
			if err != nil {
				return NeedsHumanPage{}, err
			}
		} else if !errors.Is(err, sql.ErrNoRows) {
			return NeedsHumanPage{}, fmt.Errorf("read needs human evidence event: %w", err)
		}
		items = append(items, item)
	}
	result := NeedsHumanPage{Items: items}
	if len(items) <= page.Limit {
		return result, nil
	}
	result.HasMore = true
	result.Items = items[:page.Limit]
	last := result.Items[len(result.Items)-1]
	result.NextCursor, err = encodeDashboardCursor(dashboardCursor{Kind: "needs_human", RequiredAt: stamp(last.RequiredAt), ChangeID: string(last.Change.ID)})
	if err != nil {
		return NeedsHumanPage{}, err
	}
	return result, nil
}

// ListArtifactObservations 返回 bounded ArtifactRef 与内容身份摘要。
func (s *Store) ListArtifactObservations(ctx context.Context, changeID domain.ChangeID, limit int) ([]ArtifactObservation, error) {
	if limit <= 0 || limit > maxDashboardObservationItems {
		limit = maxDashboardObservationItems
	}
	if _, err := readChange(ctx, s.db, changeID); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT r.artifact_ref_id, r.artifact_id, r.role, r.ordinal, r.created_at, r.kind, r.schema_version, r.summary, r.source_revision, a.byte_length, a.media_type FROM t_artifact_refs r JOIN t_artifacts a ON a.artifact_id = r.artifact_id WHERE r.change_id = ? ORDER BY r.created_at, r.artifact_ref_id LIMIT ?`, changeID, limit)
	if err != nil {
		return nil, fmt.Errorf("list artifact observations: %w", err)
	}
	result := make([]ArtifactObservation, 0)
	for rows.Next() {
		var ref ArtifactObservation
		var created string
		if err := rows.Scan(&ref.Ref.ID, &ref.Ref.ArtifactID, &ref.Ref.Role, &ref.Ref.Ordinal, &created, &ref.Ref.Kind, &ref.Ref.SchemaVersion, &ref.Ref.Summary, &ref.Ref.SourceRevision, &ref.ByteLength, &ref.MediaType); err != nil {
			return nil, fmt.Errorf("scan artifact observation: %w", err)
		}
		ref.Ref.ChangeID = changeID
		result = append(result, ref)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read artifact observations: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close artifact observations: %w", err)
	}
	for index := range result {
		if err := loadArtifactObservationLinks(ctx, s.db, &result[index].Ref); err != nil {
			return nil, err
		}
	}
	return result, nil
}

func loadArtifactObservationLinks(ctx context.Context, db *sql.DB, ref *domain.ArtifactRef) error {
	rows, err := db.QueryContext(ctx, `SELECT relation, linked_artifact_ref_id FROM t_artifact_ref_links WHERE artifact_ref_id = ? ORDER BY relation, ordinal`, ref.ID)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return nil
		}
		return fmt.Errorf("list artifact observation links: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var role string
		var value domain.ArtifactRefID
		if err := rows.Scan(&role, &value); err != nil {
			return err
		}
		if role == "input" {
			ref.InputArtifactRefIDs = append(ref.InputArtifactRefIDs, value)
		} else if role == "raw_log" {
			ref.RawLogArtifactRefIDs = append(ref.RawLogArtifactRefIDs, value)
		}
	}
	return rows.Err()
}

// ListWorkerHealth 返回 Worker 的安全注册摘要，不计算 freshness 结论。
func (s *Store) ListWorkerHealth(ctx context.Context) ([]WorkerHealth, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT worker_id, protocol_version, status, capabilities_json, last_heartbeat_at FROM t_worker_instances ORDER BY worker_id`)
	if err != nil {
		return nil, fmt.Errorf("list worker health: %w", err)
	}
	defer rows.Close()
	workers := make([]WorkerHealth, 0)
	for rows.Next() {
		var worker WorkerHealth
		var encoded string
		var heartbeat sql.NullString
		if err := rows.Scan(&worker.WorkerID, &worker.ProtocolVersion, &worker.Status, &encoded, &heartbeat); err != nil {
			return nil, fmt.Errorf("scan worker health: %w", err)
		}
		if err := json.Unmarshal([]byte(encoded), &worker.Capabilities); err != nil {
			return nil, fmt.Errorf("decode worker health capabilities: %w", err)
		}
		if heartbeat.Valid {
			value, err := parseStamp(heartbeat.String)
			if err != nil {
				return nil, err
			}
			worker.LastHeartbeatAt = &value
		}
		workers = append(workers, worker)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read worker health: %w", err)
	}
	return workers, nil
}
