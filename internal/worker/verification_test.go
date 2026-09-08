package worker

import (
	"context"
	"os"
	"testing"

	workercontract "github.com/disturb-yy/keystone/contracts/worker"
)

func TestCommandVerifierStopsAfterFailure(t *testing.T) {
	assignment := workercontract.VerificationAssignment{
		IntentID:              "intent",
		CandidateTreeIdentity: "tree",
		InputRevision:         "revision",
		Commands: []workercontract.VerificationCommand{
			{Name: "first", Argv: []string{os.Args[0], "-test.run=TestVerificationHelperProcess", "--", "fail"}, TimeoutSeconds: 10},
			{Name: "second", Argv: []string{os.Args[0], "-test.run=TestVerificationHelperProcess", "--", "pass"}, TimeoutSeconds: 10},
		},
		Criteria: []workercontract.AcceptanceCriterionRef{{TicketID: "ticket", Ordinal: 1, TextSHA256: "text"}},
	}
	report, err := (CommandVerifier{}).Verify(context.Background(), assignment, t.TempDir(), []string{"PATH=" + os.Getenv("PATH"), "GIT_DIR=/must-not-pass"})
	if err != nil {
		t.Fatal(err)
	}
	if report.Commands[0].Status != "failed" || report.Commands[1].Status != "not_run" || report.Criteria[0].Outcome != "not_run" {
		t.Fatalf("verification report = %+v", report)
	}
	for _, value := range restrictedVerificationEnvironment([]string{"PATH=/bin", "GIT_DIR=/secret", "KEYSTONE_TOKEN=secret"}) {
		if value == "GIT_DIR=/secret" || value == "KEYSTONE_TOKEN=secret" {
			t.Fatalf("restricted environment leaked %q", value)
		}
	}
}

func TestVerificationHelperProcess(t *testing.T) {
	if len(os.Args) > 0 && os.Args[len(os.Args)-1] == "fail" {
		os.Exit(3)
	}
}
