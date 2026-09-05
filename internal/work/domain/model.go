package domain

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	// ProjectInitializedType 是 V1 唯一允许查询的 Project Event 类型。
	ProjectInitializedType = "ProjectInitialized"
	IntentPending          = "pending"
	IntentFailed           = "failed"
)

// ProjectID 是由 ProjectManifest 持有的稳定身份。
type ProjectID string

// NewProjectID 生成新的小写 canonical UUIDv7。
func NewProjectID() ProjectID {
	value, err := uuid.NewV7()
	if err != nil {
		panic(fmt.Sprintf("generate project UUIDv7: %v", err))
	}
	return ProjectID(value.String())
}

// Validate 检查 ProjectID 的 V1 表达。
func (id ProjectID) Validate() error {
	parsed, err := uuid.Parse(string(id))
	if err != nil || parsed.Version() != 7 || parsed.Variant() != uuid.RFC4122 || parsed.String() != string(id) {
		return fmt.Errorf("%w: project_id must be lowercase canonical UUIDv7", ErrManifestInvalid)
	}
	return nil
}

// RepositoryIdentity 表示一个 Project 的稳定 Repository 身份。
type RepositoryIdentity struct{ ProjectID ProjectID }

// RepositoryBinding 表示当前物理、非 bare Git 主工作树根。
type RepositoryBinding struct {
	Root         string
	ManifestPath string
}

// Validate 检查 Binding 的路径表达。
func (b RepositoryBinding) Validate() error {
	if b.Root == "" || !filepath.IsAbs(b.Root) || filepath.Clean(b.Root) != b.Root {
		return fmt.Errorf("%w: repository binding root must be normalized absolute path", ErrInvalidRequest)
	}
	if b.ManifestPath == "" || !filepath.IsAbs(b.ManifestPath) {
		return fmt.Errorf("%w: manifest path must be absolute", ErrInvalidRequest)
	}
	return nil
}

// ProjectManifest 是 Repository 中严格版本化的 Project 身份表达。
type ProjectManifest struct {
	Version   int
	ProjectID ProjectID
}

// Validate 检查 Manifest 的业务字段。
func (m ProjectManifest) Validate() error {
	if m.Version != 1 {
		return fmt.Errorf("%w: unsupported manifest version", ErrManifestInvalid)
	}
	return m.ProjectID.Validate()
}

// Project 是当前 LocalStateRoot 内的权威 Project 快照。
type Project struct {
	Identity  RepositoryIdentity
	Binding   RepositoryBinding
	CreatedAt time.Time
}

// ProjectInitialized 是 Project 首次成为权威记录时的不可变事实。
type ProjectInitialized struct {
	EventID    string
	ProjectID  ProjectID
	OccurredAt time.Time
}

// ProjectInitializationIntent 是 Manifest 与 SQLite 之间可恢复的中间状态。
type ProjectInitializationIntent struct {
	ID             string
	ProjectID      ProjectID
	RepositoryRoot string
	IdempotencyKey string
	Status         string
	FailureCode    string
}

// ProjectInitializationReceipt 是一次幂等初始化请求的终态记录。
type ProjectInitializationReceipt struct {
	IdempotencyKey string
	RepositoryRoot string
	ProjectID      ProjectID
	Status         string
	FailureCode    string
}

// NormalizeRoot 返回绝对路径的清洁表达。
func NormalizeRoot(path string) (string, error) {
	if strings.TrimSpace(path) == "" || !filepath.IsAbs(path) {
		return "", fmt.Errorf("%w: repository path must be absolute", ErrInvalidRequest)
	}
	return filepath.Clean(path), nil
}
