package workstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/disturb-yy/keystone/internal/infrastructure/id"
)

// VerificationSnapshotInput 是 Verify 期间由 Daemon 独立采集的 Workspace 摘要。
type VerificationSnapshotInput struct {
	Phase         string
	InputRevision string
	HeadRevision  string
	Branch        string
	ChangedFiles  []string
	DiffSHA256    string
	DiffBytes     int64
	HasUntracked  bool
	TreeIdentity  string
}

func insertVerificationSnapshotTx(ctx context.Context, tx *sql.Tx, intentID string, input VerificationSnapshotInput, created string) error {
	if input.Phase != "before" && input.Phase != "after" || strings.TrimSpace(input.InputRevision) == "" || strings.TrimSpace(input.HeadRevision) == "" || strings.TrimSpace(input.TreeIdentity) == "" || input.DiffBytes < 0 {
		return fmt.Errorf("verification snapshot input is invalid: %w", ErrVerificationConflict)
	}
	changedFiles, err := json.Marshal(input.ChangedFiles)
	if err != nil {
		return fmt.Errorf("encode verification snapshot files: %w", ErrVerificationUnavailable)
	}
	result, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO t_verification_snapshots (snapshot_id, intent_id, phase, input_revision, head_revision, branch, changed_files_json, diff_sha256, diff_bytes, has_untracked, tree_identity, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, id.New(), intentID, input.Phase, input.InputRevision, input.HeadRevision, input.Branch, string(changedFiles), input.DiffSHA256, input.DiffBytes, boolInt(input.HasUntracked), input.TreeIdentity, created)
	if err != nil {
		return fmt.Errorf("persist verification snapshot: %w", ErrVerificationUnavailable)
	}
	if input.Phase == "after" {
		if count, err := result.RowsAffected(); err == nil && count == 0 {
			var existingTree, existingHead string
			if err := tx.QueryRowContext(ctx, `SELECT tree_identity, head_revision FROM t_verification_snapshots WHERE intent_id = ? AND phase = ?`, intentID, input.Phase).Scan(&existingTree, &existingHead); err != nil {
				return fmt.Errorf("read existing verification snapshot: %w", ErrVerificationUnavailable)
			}
			if existingTree != input.TreeIdentity || existingHead != input.HeadRevision {
				return fmt.Errorf("verification snapshot changed: %w", ErrVerificationConflict)
			}
		}
	}
	return nil
}

func insertGovernanceEventTx(ctx context.Context, tx *sql.Tx, projectID, changeID, operation, entityID string, details any, occurred string) error {
	encoded, err := json.Marshal(details)
	if err != nil {
		return fmt.Errorf("encode governance event: %w", ErrVerificationUnavailable)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO t_governance_events (event_id, project_id, change_id, operation, entity_id, details_json, occurred_at) VALUES (?, ?, ?, ?, ?, ?, ?)`, id.New(), projectID, changeID, operation, entityID, string(encoded), occurred); err != nil {
		return fmt.Errorf("persist governance event: %w", ErrVerificationUnavailable)
	}
	return nil
}

// RecordVerificationSnapshot 在 Report 前记录 Daemon 观察到的 after 快照。
func (s *Store) RecordVerificationSnapshot(ctx context.Context, agentRunID string, input VerificationSnapshotInput) error {
	if ctx == nil || strings.TrimSpace(agentRunID) == "" {
		return ErrWorkerLeaseInvalid
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin verification snapshot: %w", ErrVerificationUnavailable)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()
	var intentID string
	if err := tx.QueryRowContext(ctx, `SELECT intent_id FROM t_verification_intents WHERE agent_run_id = ?`, agentRunID).Scan(&intentID); err != nil {
		return ErrWorkerLeaseInvalid
	}
	if err := insertVerificationSnapshotTx(ctx, tx, intentID, input, stamp(s.now().UTC())); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit verification snapshot: %w", ErrVerificationUnavailable)
	}
	committed = true
	return nil
}
