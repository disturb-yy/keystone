// Package governance 定义 M8 用例与 authority 之间的窄 Port。
package governance

import (
	"context"

	governancedomain "github.com/disturb-yy/keystone/internal/governance/domain"
)

// VerificationStarter 是 Intent-first Verify/FinalVerify 的持久化 Port。
type VerificationStarter interface {
	BeginVerification(context.Context, VerificationRequest) (VerificationResult, error)
}

// VerificationRequest 是 Application 层不携带 HTTP/SQL 细节的请求。
type VerificationRequest struct {
	ProjectID             string
	ChangeID              string
	TicketID              string
	ExpectedVersion       int
	RequestKey            string
	RequestDigest         string
	InputRevision         string
	CandidateTreeIdentity string
	Final                 bool
}

// VerificationResult 是 Verify Command 的稳定 Application 结果。
type VerificationResult struct {
	IntentID string
	Status   string
	Final    bool
}

// RenderMessage 暴露纯领域的受控 Commit message 渲染，具体 Git 写入仍在 Adapter。
func RenderMessage(template, changeID, ticketID, title string) (string, error) {
	return governancedomain.RenderCommitMessage(template, changeID, ticketID, title)
}
