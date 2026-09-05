package domain

import "errors"

var (
	// ErrInvalidRequest 表示请求不满足 Project Bootstrap 的输入约束。
	ErrInvalidRequest = errors.New("invalid project initialization request")
	// ErrRepositoryUnsupported 表示 Git 拓扑不属于 V1 支持范围。
	ErrRepositoryUnsupported = errors.New("repository topology is unsupported")
	// ErrManifestInvalid 表示 ProjectManifest 不符合 V1 严格格式。
	ErrManifestInvalid = errors.New("project manifest is invalid")
	// ErrManifestUnavailable 表示 Manifest 所需的文件操作暂时不可用。
	ErrManifestUnavailable = errors.New("project manifest is unavailable")
	// ErrIdempotencyConflict 表示同一幂等键绑定了不同物理 Repository。
	ErrIdempotencyConflict = errors.New("idempotency key conflicts with repository")
	// ErrProjectIdentityConflict 表示 Project 身份或活动 Binding 不一致。
	ErrProjectIdentityConflict = errors.New("project identity conflicts with repository binding")
	// ErrProjectNotFound 表示查询的 Project 不存在。
	ErrProjectNotFound = errors.New("project not found")
	// ErrInternal 表示无法安全分类的持久化完整性错误。
	ErrInternal = errors.New("project initialization integrity failure")
)
