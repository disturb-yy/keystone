package planning

import (
	"errors"
	"strings"
	"testing"

	"github.com/disturb-yy/keystone/internal/work/domain"
)

func TestStrictTicketDraftDecoderAcceptsCanonicalDraftAndRejectsLeakyShapes(t *testing.T) {
	body := []byte(`{"schema_version":"keystone.ticket-draft.v1","tickets":[{"generation_key":"root","title":" Root ticket ","scope":" internal/work ","acceptance_criteria":[" done ","review"],"blocked_by":[]},{"generation_key":"child","title":"Child","scope":"internal/planning","acceptance_criteria":["strict"],"blocked_by":["root"]}]}`)
	candidate, err := DecodeTicketDraft(body)
	if err != nil {
		t.Fatal(err)
	}
	if candidate.Tickets[0].Title != "Root ticket" || candidate.Tickets[0].AcceptanceCriteria[0] != "done" || len(candidate.Tickets[1].BlockedBy) != 1 {
		t.Fatalf("candidate = %+v", candidate)
	}
	for _, test := range []struct {
		name string
		body string
		want ErrorClass
	}{
		{"unknown field", `{"schema_version":"keystone.ticket-draft.v1","tickets":[],"extra":true}`, ErrorClassDecodeInvalid},
		{"repeated field", `{"schema_version":"keystone.ticket-draft.v1","schema_version":"keystone.ticket-draft.v1","tickets":[]}`, ErrorClassDecodeInvalid},
		{"missing blocked by", `{"schema_version":"keystone.ticket-draft.v1","tickets":[{"generation_key":"one","title":"One","scope":"scope","acceptance_criteria":["done"]}]}`, ErrorClassSchemaInvalid},
		{"trailing value", `{"schema_version":"keystone.ticket-draft.v1","tickets":[]} {}`, ErrorClassDecodeInvalid},
		{"invalid utf8", "{\xff}", ErrorClassDecodeInvalid},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := DecodeTicketDraft([]byte(test.body))
			if ClassifyError(err) != test.want || !errors.Is(err, domain.ErrTicketDraftInvalid) {
				t.Fatalf("error = %v class=%q, want class=%q and ErrTicketDraftInvalid", err, ClassifyError(err), test.want)
			}
		})
	}
}

func TestTicketDraftValidatorRejectsBoundsReferencesAndCycles(t *testing.T) {
	base := func() StructuredTicketDraft {
		return StructuredTicketDraft{SchemaVersion: TicketDraftSchemaV1, Tickets: []StructuredTicket{{GenerationKey: "one", Title: "One", Scope: "scope", AcceptanceCriteria: []string{"done"}, BlockedBy: []string{}}}}
	}
	tests := []struct {
		name   string
		mutate func(*StructuredTicketDraft)
		want   ErrorClass
	}{
		{"bad key", func(d *StructuredTicketDraft) { d.Tickets[0].GenerationKey = "_one" }, ErrorClassSchemaInvalid},
		{"unknown blocker", func(d *StructuredTicketDraft) { d.Tickets[0].BlockedBy = []string{"missing"} }, ErrorClassSchemaInvalid},
		{"duplicate criterion", func(d *StructuredTicketDraft) { d.Tickets[0].AcceptanceCriteria = []string{"done", "done"} }, ErrorClassSchemaInvalid},
		{"oversized scope", func(d *StructuredTicketDraft) { d.Tickets[0].Scope = strings.Repeat("x", MaxTicketScopeBytes+1) }, ErrorClassLimitExceeded},
		{"cycle", func(d *StructuredTicketDraft) {
			d.Tickets = []StructuredTicket{{GenerationKey: "one", Title: "One", Scope: "scope", AcceptanceCriteria: []string{"done"}, BlockedBy: []string{"two"}}, {GenerationKey: "two", Title: "Two", Scope: "scope", AcceptanceCriteria: []string{"done"}, BlockedBy: []string{"one"}}}
		}, ErrorClassSchemaInvalid},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			draft := base()
			test.mutate(&draft)
			_, err := NewTicketDraftValidator().Validate(draft)
			if ClassifyError(err) != test.want || !errors.Is(err, domain.ErrTicketDraftInvalid) {
				t.Fatalf("error = %v class=%q, want class=%q", err, ClassifyError(err), test.want)
			}
		})
	}
}

func TestBuildTicketizePromptIsDeterministicAndDoesNotExposeHostDetails(t *testing.T) {
	input := TicketGenerationInput{BaseRevision: strings.Repeat("a", 40), PlanArtifactID: "plan-ref", ProjectContext: validProjectContext(), Plan: PlanPayload{Steps: []PlanStep{{ID: "step", Summary: "Implement", Paths: []string{"internal/planning"}}}, Verification: []string{"go test ./..."}}}
	first, err := BuildTicketizePrompt(input)
	if err != nil {
		t.Fatal(err)
	}
	second, err := BuildTicketizePrompt(input)
	if err != nil || first != second {
		t.Fatalf("prompt is not deterministic: %v", err)
	}
	for _, forbidden := range []string{"/home/jadon", "DATABASE_URL", "WORKER_PROTOCOL_SECRET"} {
		if strings.Contains(first, forbidden) {
			t.Fatalf("prompt contains %q", forbidden)
		}
	}
	if !strings.Contains(first, TicketDraftSchemaV1) || !strings.Contains(first, input.BaseRevision) {
		t.Fatalf("prompt misses fixed contract: %s", first)
	}
}
