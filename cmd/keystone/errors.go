package main

import "fmt"

// ErrorCategory 是 CLI 对外报告的稳定错误分类。
type ErrorCategory string

const (
	ErrorMetadataMissing     ErrorCategory = "metadata_missing"
	ErrorMetadataInvalid     ErrorCategory = "metadata_invalid"
	ErrorEndpointUnreachable ErrorCategory = "endpoint_unreachable"
	ErrorHealthNotReady      ErrorCategory = "health_not_ready"
	ErrorStatusUnavailable   ErrorCategory = "status_unavailable"
	ErrorInstanceMismatch    ErrorCategory = "instance_mismatch"
	ErrorStopRejected        ErrorCategory = "stop_rejected"
	ErrorInvalidResponse     ErrorCategory = "invalid_response"
	ErrorDaemonExecutable    ErrorCategory = "daemon_executable_unavailable"
	ErrorDaemonStartFailed   ErrorCategory = "daemon_start_failed"
	ErrorDaemonStartTimeout  ErrorCategory = "daemon_start_timeout"
	ErrorProjectInitFailed   ErrorCategory = "project_init_failed"
)

// CLIError 表示 CLI 已分类的用户可诊断错误，并保留底层错误链供测试或调用方检查。
type CLIError struct {
	Category ErrorCategory
	Message  string
	Cause    error
}

func newCLIError(category ErrorCategory, message string, cause error) *CLIError {
	return &CLIError{Category: category, Message: message, Cause: cause}
}

func (e *CLIError) Error() string {
	if e == nil {
		return ""
	}
	if e.Cause == nil {
		return fmt.Sprintf("%s: %s", e.Category, e.Message)
	}
	return fmt.Sprintf("%s: %s: %v", e.Category, e.Message, e.Cause)
}

func (e *CLIError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}
