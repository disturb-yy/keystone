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
	// ErrChangeNotFound 表示 Change 不存在。
	ErrChangeNotFound = errors.New("change not found")
	// ErrArtifactNotFound 表示 ArtifactRef 或 Artifact 不存在。
	ErrArtifactNotFound = errors.New("artifact not found")
	// ErrArtifactUnavailable 表示 Artifact 内容缺失或摘要校验失败。
	ErrArtifactUnavailable = errors.New("artifact is unavailable")
	// ErrRepositoryDirty 表示源 Repository 存在未提交变化。
	ErrRepositoryDirty = errors.New("repository is dirty")
	// ErrBaseRevisionUnavailable 表示 HEAD 不能解析为完整提交。
	ErrBaseRevisionUnavailable = errors.New("base revision is unavailable")
	// ErrSourceSnapshotUnstable 表示双读源快照期间 HEAD 发生变化。
	ErrSourceSnapshotUnstable = errors.New("source snapshot is unstable")
	// ErrChangeVersionConflict 表示命令使用了过期 ChangeVersion。
	ErrChangeVersionConflict = errors.New("change version conflicts")
	// ErrLifecycleTransitionInvalid 表示生命周期控制命令不适用于当前状态。
	ErrLifecycleTransitionInvalid = errors.New("lifecycle transition is invalid")
	// ErrHumanDecisionRequired 表示当前操作只能通过 HumanDecision 执行。
	ErrHumanDecisionRequired = errors.New("human decision is required")
	// ErrUnavailable 表示本机依赖暂时不可用。
	ErrUnavailable = errors.New("operation is temporarily unavailable")
)
