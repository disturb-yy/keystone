package worker

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"os"
	"os/exec"
	"strings"
	"time"

	workercontract "github.com/disturb-yy/keystone/contracts/worker"
)

// VerificationExecutor 是只读验证执行 seam；它不能访问 Daemon authority 或执行 Git 写命令。
type VerificationExecutor interface {
	Verify(context.Context, workercontract.VerificationAssignment, string, []string) (workercontract.VerificationReport, error)
}

// CommandVerifier 以固定 argv、固定 cwd 和受限环境运行 Manifest 命令。
type CommandVerifier struct{}

// Verify 执行 ordered commands；首个失败后仍补齐后续 not_run 结果。
func (CommandVerifier) Verify(ctx context.Context, assignment workercontract.VerificationAssignment, workspace string, environment []string) (workercontract.VerificationReport, error) {
	if ctx == nil || workspace == "" || len(assignment.Commands) == 0 {
		return workercontract.VerificationReport{}, errors.New("verification input is invalid")
	}
	report := workercontract.VerificationReport{IntentID: assignment.IntentID, CandidateTreeIdentity: assignment.CandidateTreeIdentity, AfterRevision: assignment.InputRevision, ReviewSummary: "command evidence collected; acceptance criteria require independent review", Commands: make([]workercontract.VerificationCommandResult, 0, len(assignment.Commands)), Criteria: make([]workercontract.VerificationCriterionResult, 0, len(assignment.Criteria))}
	failed := false
	for index, command := range assignment.Commands {
		result := workercontract.VerificationCommandResult{Ordinal: index + 1, Name: command.Name}
		if failed {
			result.Status = "not_run"
			report.Commands = append(report.Commands, result)
			continue
		}
		commandResult, err := runVerificationCommand(ctx, workspace, command, environment)
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				result.Status = "timed_out"
			} else {
				result.Status = "failed"
			}
			failed = true
		} else {
			result.Status = "passed"
		}
		result.ExitCode = commandResult.exitCode
		result.Stdout = commandResult.stdout
		result.Stderr = commandResult.stderr
		report.Commands = append(report.Commands, result)
	}
	for _, criterion := range assignment.Criteria {
		outcome := "human_required"
		if failed {
			outcome = "not_run"
		}
		report.Criteria = append(report.Criteria, workercontract.VerificationCriterionResult{TicketID: criterion.TicketID, Ordinal: criterion.Ordinal, TextSHA256: criterion.TextSHA256, Outcome: outcome})
	}
	if failed {
		report.Outcome = "failed"
	} else {
		report.Outcome = "succeeded"
	}
	return report, nil
}

type verificationCommandResult struct {
	exitCode *int
	stdout   workercontract.VerificationEvidence
	stderr   workercontract.VerificationEvidence
}

func runVerificationCommand(parent context.Context, workspace string, command workercontract.VerificationCommand, environment []string) (verificationCommandResult, error) {
	if command.TimeoutSeconds < 1 || command.TimeoutSeconds > 1800 || len(command.Argv) == 0 {
		return verificationCommandResult{}, errors.New("verification command is invalid")
	}
	ctx, cancel := context.WithTimeout(parent, time.Duration(command.TimeoutSeconds)*time.Second)
	defer cancel()
	process := exec.CommandContext(ctx, command.Argv[0], command.Argv[1:]...)
	process.Dir = workspace
	process.Env = restrictedVerificationEnvironment(environment)
	stdout, stderr := &boundedVerificationWriter{}, &boundedVerificationWriter{}
	process.Stdout, process.Stderr = stdout, stderr
	err := process.Run()
	code := -1
	if process.ProcessState != nil {
		code = process.ProcessState.ExitCode()
	}
	result := verificationCommandResult{exitCode: &code, stdout: verificationEvidence(stdout), stderr: verificationEvidence(stderr)}
	if ctx.Err() != nil {
		return result, ctx.Err()
	}
	if err != nil {
		return result, err
	}
	return result, nil
}

func restrictedVerificationEnvironment(environment []string) []string {
	if environment == nil {
		environment = os.Environ()
	}
	allowed := map[string]bool{"PATH": true, "HOME": true, "TMPDIR": true, "LANG": true, "LC_ALL": true}
	result := make([]string, 0, len(environment))
	for _, value := range environment {
		name, _, ok := strings.Cut(value, "=")
		if ok && allowed[name] {
			result = append(result, value)
		}
	}
	return result
}

type boundedVerificationWriter struct {
	content bytes.Buffer
	total   int64
	limit   bool
}

func (w *boundedVerificationWriter) Write(value []byte) (int, error) {
	w.total += int64(len(value))
	remaining := (256 << 10) - w.content.Len()
	if remaining > 0 {
		if len(value) > remaining {
			_, _ = w.content.Write(value[:remaining])
			w.limit = true
		} else {
			_, _ = w.content.Write(value)
		}
	} else {
		w.limit = true
	}
	return len(value), nil
}

func verificationEvidence(writer *boundedVerificationWriter) workercontract.VerificationEvidence {
	content := writer.content.Bytes()
	var digest string
	if len(content) > 0 {
		hash := sha256.Sum256(content)
		digest = hex.EncodeToString(hash[:])
	}
	return workercontract.VerificationEvidence{ContentBase64: base64.StdEncoding.EncodeToString(content), SHA256: digest, SizeBytes: writer.total, Truncated: writer.limit}
}
