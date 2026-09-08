package planning

import "errors"

// ErrorClass 是 Planning 边界向调用方暴露的稳定失败分类。
type ErrorClass string

const (
	ErrorClassInputInvalid     ErrorClass = "input_invalid"
	ErrorClassDecodeInvalid    ErrorClass = "decode_invalid"
	ErrorClassSchemaInvalid    ErrorClass = "schema_invalid"
	ErrorClassLimitExceeded    ErrorClass = "limit_exceeded"
	ErrorClassRevisionMismatch ErrorClass = "revision_mismatch"
	ErrorClassRuntimeFailed    ErrorClass = "runtime_failed"
	ErrorClassRuntimeTimeout   ErrorClass = "runtime_timeout"
)

var (
	// ErrDispatchFenced 表示 Assignment 在 authority 复检时已被 Pause、Cancel 或并发调度围栏。
	ErrDispatchFenced = errors.New("planning dispatch is fenced")
	// ErrInputInvalid 表示阶段输入或 ProjectContext 不满足前置条件。
	ErrInputInvalid = errors.New("planning input is invalid")
	// ErrDecodeInvalid 表示候选不是严格的单一 JSON object。
	ErrDecodeInvalid = errors.New("planning candidate JSON is invalid")
	// ErrSchemaInvalid 表示候选字段或阶段 schema 不满足契约。
	ErrSchemaInvalid = errors.New("planning candidate schema is invalid")
	// ErrLimitExceeded 表示 Context、Artifact 或字段超过冻结上限。
	ErrLimitExceeded = errors.New("planning value exceeds limit")
	// ErrRevisionMismatch 表示候选或上游 Artifact 不属于固定 base revision。
	ErrRevisionMismatch = errors.New("planning source revision mismatches")
	// ErrRuntimeFailed 表示 Runtime 启动或非零退出。
	ErrRuntimeFailed = errors.New("planning runtime failed")
	// ErrRuntimeTimeout 表示 Runtime 执行超时。
	ErrRuntimeTimeout = errors.New("planning runtime timed out")
)

// PlanningError 在不暴露 parser、宿主路径或运行环境细节的前提下提供字段上下文。
type PlanningError struct {
	Class ErrorClass
	Field string
	cause error
}

// Error 返回稳定分类和字段，不拼接底层错误或原始候选内容。
func (e *PlanningError) Error() string {
	if e == nil {
		return "planning error"
	}
	if e.Field == "" {
		return string(e.Class)
	}
	return string(e.Class) + ": " + e.Field
}

// Unwrap 支持调用方通过 errors.Is 匹配稳定错误类别。
func (e *PlanningError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

// ClassifyError 返回已知 Planning 错误的稳定分类。
func ClassifyError(err error) ErrorClass {
	var target *PlanningError
	if errors.As(err, &target) {
		return target.Class
	}
	return ""
}

func planningError(class ErrorClass, field string) error {
	return &PlanningError{Class: class, Field: field, cause: causeForClass(class)}
}

func causeForClass(class ErrorClass) error {
	switch class {
	case ErrorClassInputInvalid:
		return ErrInputInvalid
	case ErrorClassDecodeInvalid:
		return ErrDecodeInvalid
	case ErrorClassSchemaInvalid:
		return ErrSchemaInvalid
	case ErrorClassLimitExceeded:
		return ErrLimitExceeded
	case ErrorClassRevisionMismatch:
		return ErrRevisionMismatch
	case ErrorClassRuntimeFailed:
		return ErrRuntimeFailed
	case ErrorClassRuntimeTimeout:
		return ErrRuntimeTimeout
	default:
		return nil
	}
}
