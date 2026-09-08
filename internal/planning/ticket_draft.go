package planning

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/disturb-yy/keystone/internal/work/domain"
)

const (
	// TicketDraftSchemaV1 是 Ticketize Runtime 候选的唯一支持 schema。
	TicketDraftSchemaV1         = "keystone.ticket-draft.v1"
	MaxTicketDraftTickets       = 64
	MaxTicketTitleRunes         = 256
	MaxTicketScopeBytes         = 8 << 10
	MaxTicketAcceptanceCriteria = 32
	MaxTicketAcceptanceBytes    = 8 << 10
	MaxTicketGenerationKeyBytes = 64
)

// StructuredTicketDraft 是严格解码后的 Ticket Draft，不携带 Daemon 分配的身份。
type StructuredTicketDraft struct {
	SchemaVersion string
	Tickets       []StructuredTicket
}

// StructuredTicket 是 Draft 中通过 JSON Contract 解码的单个 Ticket。
type StructuredTicket struct {
	GenerationKey      string
	Title              string
	Scope              string
	AcceptanceCriteria []string
	BlockedBy          []string
}

// TicketDraftValidator 将结构化 Draft 校验为 Work 可提交的非权威候选。
type TicketDraftValidator struct{}

// StrictTicketDraftDecoder 只接受单一 UTF-8 JSON object，并拒绝未知、重复和缺失字段。
type StrictTicketDraftDecoder struct {
	validator *TicketDraftValidator
}

// NewTicketDraftValidator 创建无外部依赖的 Draft validator。
func NewTicketDraftValidator() *TicketDraftValidator { return &TicketDraftValidator{} }

// NewStrictTicketDraftDecoder 创建严格 Draft decoder。
func NewStrictTicketDraftDecoder() *StrictTicketDraftDecoder {
	return &StrictTicketDraftDecoder{validator: NewTicketDraftValidator()}
}

type ticketDraftWire struct {
	SchemaVersion *string            `json:"schema_version"`
	Tickets       *[]ticketDraftItem `json:"tickets"`
}

type ticketDraftItem struct {
	GenerationKey      *string   `json:"generation_key"`
	Title              *string   `json:"title"`
	Scope              *string   `json:"scope"`
	AcceptanceCriteria *[]string `json:"acceptance_criteria"`
	BlockedBy          *[]string `json:"blocked_by"`
}

// Decode 解码 Draft 的结构和字段存在性，但不产生权威身份。
func (d *StrictTicketDraftDecoder) Decode(body []byte) (StructuredTicketDraft, error) {
	if d == nil {
		return StructuredTicketDraft{}, draftError(ErrorClassInputInvalid, "decoder")
	}
	var wire ticketDraftWire
	if err := strictDecodeObject(body, MaxArtifactBytes, &wire); err != nil {
		return StructuredTicketDraft{}, fmt.Errorf("%w: %w", domain.ErrTicketDraftInvalid, err)
	}
	if wire.SchemaVersion == nil {
		return StructuredTicketDraft{}, draftError(ErrorClassSchemaInvalid, "schema_version")
	}
	if wire.Tickets == nil {
		return StructuredTicketDraft{}, draftError(ErrorClassSchemaInvalid, "tickets")
	}
	draft := StructuredTicketDraft{SchemaVersion: *wire.SchemaVersion, Tickets: make([]StructuredTicket, len(*wire.Tickets))}
	for index, item := range *wire.Tickets {
		field := fmt.Sprintf("tickets[%d]", index)
		if item.GenerationKey == nil {
			return StructuredTicketDraft{}, draftError(ErrorClassSchemaInvalid, field+".generation_key")
		}
		if item.Title == nil {
			return StructuredTicketDraft{}, draftError(ErrorClassSchemaInvalid, field+".title")
		}
		if item.Scope == nil {
			return StructuredTicketDraft{}, draftError(ErrorClassSchemaInvalid, field+".scope")
		}
		if item.AcceptanceCriteria == nil {
			return StructuredTicketDraft{}, draftError(ErrorClassSchemaInvalid, field+".acceptance_criteria")
		}
		if item.BlockedBy == nil {
			return StructuredTicketDraft{}, draftError(ErrorClassSchemaInvalid, field+".blocked_by")
		}
		draft.Tickets[index] = StructuredTicket{
			GenerationKey: *item.GenerationKey, Title: *item.Title, Scope: *item.Scope,
			AcceptanceCriteria: append([]string(nil), (*item.AcceptanceCriteria)...),
			BlockedBy:          append([]string(nil), (*item.BlockedBy)...),
		}
	}
	return draft, nil
}

// DecodeAndValidate 解码并返回已经通过严格 Draft Contract 的候选。
func (d *StrictTicketDraftDecoder) DecodeAndValidate(body []byte) (domain.TicketGraphCandidate, error) {
	draft, err := d.Decode(body)
	if err != nil {
		return domain.TicketGraphCandidate{}, err
	}
	validator := d.validator
	if validator == nil {
		validator = NewTicketDraftValidator()
	}
	return validator.Validate(draft)
}

// Validate 将 Draft 校验并规范化为 Work authority 可消费的候选。
func (v *TicketDraftValidator) Validate(draft StructuredTicketDraft) (domain.TicketGraphCandidate, error) {
	if v == nil {
		return domain.TicketGraphCandidate{}, draftError(ErrorClassInputInvalid, "validator")
	}
	if draft.SchemaVersion != TicketDraftSchemaV1 {
		return domain.TicketGraphCandidate{}, draftError(ErrorClassSchemaInvalid, "schema_version")
	}
	if len(draft.Tickets) < 1 || len(draft.Tickets) > MaxTicketDraftTickets {
		return domain.TicketGraphCandidate{}, draftError(ErrorClassLimitExceeded, "tickets")
	}
	candidate := domain.TicketGraphCandidate{Tickets: make([]domain.TicketDraft, len(draft.Tickets))}
	for index, ticket := range draft.Tickets {
		candidate.Tickets[index] = domain.TicketDraft{
			GenerationKey:      ticket.GenerationKey,
			Title:              ticket.Title,
			Scope:              ticket.Scope,
			AcceptanceCriteria: append([]string(nil), ticket.AcceptanceCriteria...),
			BlockedBy:          append([]string(nil), ticket.BlockedBy...),
		}
		if !validGenerationKey(ticket.GenerationKey) {
			return domain.TicketGraphCandidate{}, draftError(ErrorClassSchemaInvalid, fieldAt("tickets", index)+".generation_key")
		}
		if !utf8.ValidString(ticket.Title) || strings.TrimSpace(ticket.Title) == "" {
			return domain.TicketGraphCandidate{}, draftError(ErrorClassSchemaInvalid, fieldAt("tickets", index)+".title")
		}
		if utf8.RuneCountInString(strings.TrimSpace(ticket.Title)) > MaxTicketTitleRunes {
			return domain.TicketGraphCandidate{}, draftError(ErrorClassLimitExceeded, fieldAt("tickets", index)+".title")
		}
		if !utf8.ValidString(ticket.Scope) || strings.TrimSpace(ticket.Scope) == "" {
			return domain.TicketGraphCandidate{}, draftError(ErrorClassSchemaInvalid, fieldAt("tickets", index)+".scope")
		}
		if len([]byte(strings.TrimSpace(ticket.Scope))) > MaxTicketScopeBytes {
			return domain.TicketGraphCandidate{}, draftError(ErrorClassLimitExceeded, fieldAt("tickets", index)+".scope")
		}
		if len(ticket.AcceptanceCriteria) < 1 || len(ticket.AcceptanceCriteria) > MaxTicketAcceptanceCriteria {
			return domain.TicketGraphCandidate{}, draftError(ErrorClassLimitExceeded, fieldAt("tickets", index)+".acceptance_criteria")
		}
		for criterionIndex, criterion := range ticket.AcceptanceCriteria {
			if !utf8.ValidString(criterion) || strings.TrimSpace(criterion) == "" {
				return domain.TicketGraphCandidate{}, draftError(ErrorClassSchemaInvalid, fmt.Sprintf("tickets[%d].acceptance_criteria[%d]", index, criterionIndex))
			}
			if len([]byte(strings.TrimSpace(criterion))) > MaxTicketAcceptanceBytes {
				return domain.TicketGraphCandidate{}, draftError(ErrorClassLimitExceeded, fmt.Sprintf("tickets[%d].acceptance_criteria[%d]", index, criterionIndex))
			}
		}
		if len(ticket.BlockedBy) > MaxTicketDraftTickets-1 {
			return domain.TicketGraphCandidate{}, draftError(ErrorClassLimitExceeded, fieldAt("tickets", index)+".blocked_by")
		}
	}
	normalized, err := domain.NormalizeTicketGraphCandidate(candidate)
	if err != nil {
		return domain.TicketGraphCandidate{}, fmt.Errorf("%w: %w", domain.ErrTicketDraftInvalid, draftClassError(err))
	}
	return normalized, nil
}

// DecodeTicketDraft 是严格解码并校验 Draft 的便捷函数。
func DecodeTicketDraft(body []byte) (domain.TicketGraphCandidate, error) {
	return NewStrictTicketDraftDecoder().DecodeAndValidate(body)
}

// TicketGenerator 是 Ticketize 阶段的候选生成端口，不拥有生命周期或持久化权限。
type TicketGenerator interface {
	Generate(context.Context, TicketGenerationInput) ([]byte, error)
}

// TicketGenerationInput 是固定 base revision 下交给 Generator 的最小输入。
type TicketGenerationInput struct {
	BaseRevision   string
	Plan           PlanPayload
	PlanArtifactID string
	ProjectContext ProjectContext
}

type ticketizePrompt struct {
	Stage          Stage               `json:"stage"`
	SchemaVersion  string              `json:"schema_version"`
	BaseRevision   string              `json:"base_revision"`
	ProjectContext ProjectContext      `json:"project_context"`
	Plan           PlanPayload         `json:"plan"`
	PlanArtifactID string              `json:"plan_artifact_id"`
	OutputContract ticketDraftContract `json:"output_contract"`
}

type ticketDraftContract struct {
	SchemaVersion     string   `json:"schema_version"`
	RequiredKeys      []string `json:"required_keys"`
	MaxTickets        int      `json:"max_tickets"`
	MaxTitleRunes     int      `json:"max_title_runes"`
	MaxScopeBytes     int      `json:"max_scope_bytes"`
	MaxCriteria       int      `json:"max_acceptance_criteria"`
	MaxCriterionBytes int      `json:"max_acceptance_criterion_bytes"`
}

// BuildTicketizePrompt 构造不含宿主路径、凭据或数据库细节的 Ticketize 输入。
func BuildTicketizePrompt(input TicketGenerationInput) (string, error) {
	if !validRevision(input.BaseRevision) || strings.TrimSpace(input.PlanArtifactID) == "" {
		return "", draftError(ErrorClassInputInvalid, "ticketize_input")
	}
	if err := NewSchemaValidator().ValidateProjectContext(input.ProjectContext); err != nil {
		return "", err
	}
	encoded, err := json.Marshal(ticketizePrompt{
		Stage: StageTicketize, SchemaVersion: TicketDraftSchemaV1, BaseRevision: input.BaseRevision,
		ProjectContext: cloneProjectContext(input.ProjectContext), Plan: clonePlan(input.Plan), PlanArtifactID: input.PlanArtifactID,
		OutputContract: ticketDraftContract{SchemaVersion: TicketDraftSchemaV1, RequiredKeys: []string{"schema_version", "tickets"}, MaxTickets: MaxTicketDraftTickets, MaxTitleRunes: MaxTicketTitleRunes, MaxScopeBytes: MaxTicketScopeBytes, MaxCriteria: MaxTicketAcceptanceCriteria, MaxCriterionBytes: MaxTicketAcceptanceBytes},
	})
	if err != nil {
		return "", draftError(ErrorClassInputInvalid, "ticketize_prompt")
	}
	return "Produce exactly one UTF-8 JSON object for Ticketize. Return no prose, markdown, lifecycle claim, or generation_key explanation.\n" + string(encoded), nil
}

func validGenerationKey(value string) bool {
	if len(value) < 1 || len(value) > MaxTicketGenerationKeyBytes || strings.ToLower(value) != value {
		return false
	}
	for index, char := range value {
		if !((char >= 'a' && char <= 'z') || (char >= '0' && char <= '9') || char == '.' || char == '_' || char == '-') {
			return false
		}
		if index == 0 && !((char >= 'a' && char <= 'z') || (char >= '0' && char <= '9')) {
			return false
		}
	}
	return true
}

func draftError(class ErrorClass, field string) error {
	return fmt.Errorf("%w: %w", domain.ErrTicketDraftInvalid, planningError(class, field))
}

func draftClassError(err error) error {
	if ClassifyError(err) != "" {
		return err
	}
	return planningError(ErrorClassSchemaInvalid, "tickets")
}

// MarshalTicketDraft 将候选编码为 Generator 使用的稳定 Draft JSON。
func MarshalTicketDraft(candidate domain.TicketGraphCandidate) ([]byte, error) {
	normalized, err := domain.NormalizeTicketGraphCandidate(candidate)
	if err != nil {
		return nil, err
	}
	wire := struct {
		SchemaVersion string            `json:"schema_version"`
		Tickets       []ticketDraftItem `json:"tickets"`
	}{SchemaVersion: TicketDraftSchemaV1, Tickets: make([]ticketDraftItem, len(normalized.Tickets))}
	for index, ticket := range normalized.Tickets {
		criteria := append([]string(nil), ticket.AcceptanceCriteria...)
		blockedBy := append([]string(nil), ticket.BlockedBy...)
		wire.Tickets[index] = ticketDraftItem{GenerationKey: &ticket.GenerationKey, Title: &ticket.Title, Scope: &ticket.Scope, AcceptanceCriteria: &criteria, BlockedBy: &blockedBy}
	}
	return json.Marshal(wire)
}
