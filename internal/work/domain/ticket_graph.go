package domain

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	// TicketDependencyBlockedBy 是 V1 唯一允许的 Ticket 依赖方向。
	TicketDependencyBlockedBy = "BLOCKED_BY"

	maxCanonicalTickets         = 64
	maxTicketGenerationKeyBytes = 64
	maxTicketTitleRunes         = 256
	maxTicketScopeBytes         = 8 << 10
	maxTicketAcceptanceCriteria = 32
	maxTicketAcceptanceBytes    = 8 << 10
)

// TicketGraphID 是一次 Change 的不可变 Canonical Ticket Graph 身份。
type TicketGraphID string

// TicketID 是 Daemon 分配的 Canonical Ticket 身份。
type TicketID string

// CanonicalTicketID 是 TicketID 的语义别名，便于调用方表达公开身份。
type CanonicalTicketID = TicketID

// TicketAcceptanceCriterion 是一个有序、不可变的 Ticket 验收标准。
type TicketAcceptanceCriterion struct {
	Ordinal int
	Text    string
}

// TicketDraft 是 Ticketize Runtime 候选中的一个结构化 Ticket。
// GenerationKey 只用于本次 Draft 内部关联，不能作为 Canonical Ticket 身份对外公布。
type TicketDraft struct {
	GenerationKey      string
	Title              string
	Scope              string
	AcceptanceCriteria []string
	BlockedBy          []string
}

// TicketGraphCandidate 是经过严格 Draft decoder/validator 后的非权威候选。
type TicketGraphCandidate struct {
	Tickets []TicketDraft
}

// CanonicalTicket 是成功 Ticketize 后由 Daemon 分配身份的不可变 Ticket。
type CanonicalTicket struct {
	ID                 TicketID
	GraphID            TicketGraphID
	Ordinal            int
	Title              string
	Scope              string
	AcceptanceCriteria []TicketAcceptanceCriterion
}

// TicketDependency 表示 dependent ticket 被 blocker ticket 阻塞。
type TicketDependency struct {
	GraphID           TicketGraphID
	DependentTicketID TicketID
	BlockerTicketID   TicketID
	Kind              string
}

// CanonicalTicketGraph 是一个 Change 唯一且不可变的 Ticket Graph。
type CanonicalTicketGraph struct {
	ID                  TicketGraphID
	ProjectID           ProjectID
	ChangeID            ChangeID
	BaseRevision        string
	PlanArtifactRef     ArtifactRef
	DraftArtifactRef    ArtifactRef
	TicketizeAgentRunID AgentRunID
	GeneratorName       string
	GeneratorVersion    string
	CreatedAt           time.Time
	Tickets             []CanonicalTicket
	Dependencies        []TicketDependency
}

// Validate 检查 Ticket Graph 的身份、来源、Ticket 顺序和依赖闭包。
func (g CanonicalTicketGraph) Validate() error {
	if err := validateUUIDv7(string(g.ID), "ticket_graph_id"); err != nil {
		return err
	}
	if err := g.ProjectID.Validate(); err != nil {
		return fmt.Errorf("ticket graph project: %w", err)
	}
	if err := validateUUIDv7(string(g.ChangeID), "change_id"); err != nil {
		return err
	}
	if !validObjectID(g.BaseRevision) || g.TicketizeAgentRunID == "" {
		return fmt.Errorf("%w: ticket graph authority fields are invalid", ErrInvalidRequest)
	}
	if err := validateUUIDv7(string(g.TicketizeAgentRunID), "ticketize_agent_run_id"); err != nil {
		return err
	}
	if err := validateGraphSourceRef(g.PlanArtifactRef, g.ChangeID, "plan"); err != nil {
		return err
	}
	if err := validateGraphSourceRef(g.DraftArtifactRef, g.ChangeID, "draft"); err != nil {
		return err
	}
	if g.PlanArtifactRef.SourceRevision != g.BaseRevision || g.DraftArtifactRef.SourceRevision != g.BaseRevision {
		return fmt.Errorf("%w: ticket graph artifact source revision is inconsistent", ErrInvalidRequest)
	}
	if strings.TrimSpace(g.GeneratorName) == "" || strings.TrimSpace(g.GeneratorVersion) == "" || g.CreatedAt.IsZero() {
		return fmt.Errorf("%w: ticket graph generator metadata is invalid", ErrInvalidRequest)
	}
	if len(g.Tickets) < 1 || len(g.Tickets) > maxCanonicalTickets {
		return fmt.Errorf("%w: ticket graph ticket count is invalid", ErrInvalidRequest)
	}
	tickets := make(map[TicketID]struct{}, len(g.Tickets))
	for index, ticket := range g.Tickets {
		if err := validateUUIDv7(string(ticket.ID), "ticket_id"); err != nil {
			return err
		}
		if err := validateUUIDv7(string(ticket.GraphID), "ticket_graph_id"); err != nil || ticket.GraphID != g.ID {
			return fmt.Errorf("%w: ticket graph ownership is invalid", ErrInvalidRequest)
		}
		if ticket.Ordinal != index+1 || strings.TrimSpace(ticket.Title) != ticket.Title || ticket.Title == "" || utf8.RuneCountInString(ticket.Title) > maxTicketTitleRunes || !utf8.ValidString(ticket.Title) {
			return fmt.Errorf("%w: ticket title or ordinal is invalid", ErrInvalidRequest)
		}
		if strings.TrimSpace(ticket.Scope) != ticket.Scope || ticket.Scope == "" || len([]byte(ticket.Scope)) > maxTicketScopeBytes || !utf8.ValidString(ticket.Scope) {
			return fmt.Errorf("%w: ticket scope is invalid", ErrInvalidRequest)
		}
		if _, exists := tickets[ticket.ID]; exists {
			return fmt.Errorf("%w: duplicate ticket id", ErrInvalidRequest)
		}
		tickets[ticket.ID] = struct{}{}
		if len(ticket.AcceptanceCriteria) < 1 || len(ticket.AcceptanceCriteria) > maxTicketAcceptanceCriteria {
			return fmt.Errorf("%w: ticket acceptance criteria count is invalid", ErrInvalidRequest)
		}
		seenCriteria := make(map[string]struct{}, len(ticket.AcceptanceCriteria))
		for criterionIndex, criterion := range ticket.AcceptanceCriteria {
			if criterion.Ordinal != criterionIndex+1 || strings.TrimSpace(criterion.Text) != criterion.Text || criterion.Text == "" || len([]byte(criterion.Text)) > maxTicketAcceptanceBytes || !utf8.ValidString(criterion.Text) {
				return fmt.Errorf("%w: ticket acceptance criterion is invalid", ErrInvalidRequest)
			}
			if _, exists := seenCriteria[criterion.Text]; exists {
				return fmt.Errorf("%w: duplicate ticket acceptance criterion", ErrInvalidRequest)
			}
			seenCriteria[criterion.Text] = struct{}{}
		}
	}
	edges := make(map[TicketID][]TicketID, len(g.Tickets))
	seenEdges := make(map[string]struct{}, len(g.Dependencies))
	for _, dependency := range g.Dependencies {
		if dependency.GraphID != g.ID || dependency.Kind != TicketDependencyBlockedBy {
			return fmt.Errorf("%w: ticket dependency kind or ownership is invalid", ErrInvalidRequest)
		}
		if dependency.DependentTicketID == dependency.BlockerTicketID {
			return fmt.Errorf("%w: ticket cannot block itself", ErrInvalidRequest)
		}
		if _, ok := tickets[dependency.DependentTicketID]; !ok {
			return fmt.Errorf("%w: dependent ticket is missing", ErrInvalidRequest)
		}
		if _, ok := tickets[dependency.BlockerTicketID]; !ok {
			return fmt.Errorf("%w: blocker ticket is missing", ErrInvalidRequest)
		}
		key := string(dependency.DependentTicketID) + "\x00" + string(dependency.BlockerTicketID)
		if _, exists := seenEdges[key]; exists {
			return fmt.Errorf("%w: duplicate ticket dependency", ErrInvalidRequest)
		}
		seenEdges[key] = struct{}{}
		edges[dependency.DependentTicketID] = append(edges[dependency.DependentTicketID], dependency.BlockerTicketID)
	}
	if hasTicketCycle(tickets, edges) {
		return fmt.Errorf("%w: ticket dependency graph contains a cycle", ErrInvalidRequest)
	}
	return nil
}

// StructuralFrontier 返回当前没有直接 blocker 的 Ticket ID，并按 ordinal 稳定排序。
func (g CanonicalTicketGraph) StructuralFrontier() []TicketID {
	blocked := make(map[TicketID]struct{}, len(g.Dependencies))
	for _, dependency := range g.Dependencies {
		if dependency.Kind == TicketDependencyBlockedBy {
			blocked[dependency.DependentTicketID] = struct{}{}
		}
	}
	frontier := make([]TicketID, 0, len(g.Tickets))
	for _, ticket := range g.Tickets {
		if _, exists := blocked[ticket.ID]; !exists {
			frontier = append(frontier, ticket.ID)
		}
	}
	return frontier
}

// NormalizeTicketGraphCandidate 返回去除首尾空白后的候选副本。
func NormalizeTicketGraphCandidate(candidate TicketGraphCandidate) (TicketGraphCandidate, error) {
	if err := validateTicketGraphCandidate(candidate); err != nil {
		return TicketGraphCandidate{}, err
	}
	normalized := TicketGraphCandidate{Tickets: make([]TicketDraft, len(candidate.Tickets))}
	for index, ticket := range candidate.Tickets {
		normalized.Tickets[index] = TicketDraft{
			GenerationKey: strings.TrimSpace(ticket.GenerationKey), Title: strings.TrimSpace(ticket.Title), Scope: strings.TrimSpace(ticket.Scope),
			AcceptanceCriteria: make([]string, len(ticket.AcceptanceCriteria)), BlockedBy: append([]string(nil), ticket.BlockedBy...),
		}
		for criterionIndex, criterion := range ticket.AcceptanceCriteria {
			normalized.Tickets[index].AcceptanceCriteria[criterionIndex] = strings.TrimSpace(criterion)
		}
	}
	return normalized, nil
}

func validateTicketGraphCandidate(candidate TicketGraphCandidate) error {
	if len(candidate.Tickets) < 1 || len(candidate.Tickets) > maxCanonicalTickets {
		return fmt.Errorf("%w: ticket count is invalid", ErrTicketDraftInvalid)
	}
	keys := make(map[string]struct{}, len(candidate.Tickets))
	for _, ticket := range candidate.Tickets {
		if !validTicketGenerationKey(ticket.GenerationKey) {
			return fmt.Errorf("%w: generation_key is invalid", ErrTicketDraftInvalid)
		}
		if _, exists := keys[ticket.GenerationKey]; exists {
			return fmt.Errorf("%w: generation_key is duplicated", ErrTicketDraftInvalid)
		}
		keys[ticket.GenerationKey] = struct{}{}
		if !utf8.ValidString(ticket.Title) || strings.TrimSpace(ticket.Title) == "" || utf8.RuneCountInString(strings.TrimSpace(ticket.Title)) > maxTicketTitleRunes {
			return fmt.Errorf("%w: title is invalid", ErrTicketDraftInvalid)
		}
		if !utf8.ValidString(ticket.Scope) || strings.TrimSpace(ticket.Scope) == "" || len([]byte(strings.TrimSpace(ticket.Scope))) > maxTicketScopeBytes {
			return fmt.Errorf("%w: scope is invalid", ErrTicketDraftInvalid)
		}
		if len(ticket.AcceptanceCriteria) < 1 || len(ticket.AcceptanceCriteria) > maxTicketAcceptanceCriteria {
			return fmt.Errorf("%w: acceptance_criteria count is invalid", ErrTicketDraftInvalid)
		}
		seen := make(map[string]struct{}, len(ticket.AcceptanceCriteria))
		for _, criterion := range ticket.AcceptanceCriteria {
			criterion = strings.TrimSpace(criterion)
			if !utf8.ValidString(criterion) || criterion == "" || len([]byte(criterion)) > maxTicketAcceptanceBytes {
				return fmt.Errorf("%w: acceptance_criteria item is invalid", ErrTicketDraftInvalid)
			}
			if _, exists := seen[criterion]; exists {
				return fmt.Errorf("%w: acceptance_criteria item is duplicated", ErrTicketDraftInvalid)
			}
			seen[criterion] = struct{}{}
		}
		if len(ticket.BlockedBy) > maxCanonicalTickets-1 {
			return fmt.Errorf("%w: blocked_by count is invalid", ErrTicketDraftInvalid)
		}
		seenBlockers := make(map[string]struct{}, len(ticket.BlockedBy))
		for _, blocker := range ticket.BlockedBy {
			if _, ok := keys[blocker]; ok && blocker == ticket.GenerationKey {
				return fmt.Errorf("%w: ticket cannot block itself", ErrTicketDraftInvalid)
			}
			if _, exists := seenBlockers[blocker]; exists {
				return fmt.Errorf("%w: blocked_by is duplicated", ErrTicketDraftInvalid)
			}
			seenBlockers[blocker] = struct{}{}
		}
	}
	for _, ticket := range candidate.Tickets {
		for _, blocker := range ticket.BlockedBy {
			if _, ok := keys[blocker]; !ok {
				return fmt.Errorf("%w: blocked_by references an unknown generation_key", ErrTicketDraftInvalid)
			}
		}
	}
	edges := make(map[string][]string, len(candidate.Tickets))
	for _, ticket := range candidate.Tickets {
		edges[ticket.GenerationKey] = append([]string(nil), ticket.BlockedBy...)
	}
	if hasStringCycle(keys, edges) {
		return fmt.Errorf("%w: blocked_by graph contains a cycle", ErrTicketDraftInvalid)
	}
	return nil
}

func validTicketGenerationKey(value string) bool {
	if len(value) < 1 || len(value) > maxTicketGenerationKeyBytes || strings.ToLower(value) != value {
		return false
	}
	for index, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= '0' && char <= '9') || char == '.' || char == '_' || char == '-' {
			if index == 0 && !((char >= 'a' && char <= 'z') || (char >= '0' && char <= '9')) {
				return false
			}
			continue
		}
		return false
	}
	return true
}

func hasTicketCycle(nodes map[TicketID]struct{}, edges map[TicketID][]TicketID) bool {
	state := make(map[TicketID]uint8, len(nodes))
	var visit func(TicketID) bool
	visit = func(node TicketID) bool {
		if state[node] == 1 {
			return true
		}
		if state[node] == 2 {
			return false
		}
		state[node] = 1
		for _, next := range edges[node] {
			if visit(next) {
				return true
			}
		}
		state[node] = 2
		return false
	}
	for node := range nodes {
		if visit(node) {
			return true
		}
	}
	return false
}

func hasStringCycle(nodes map[string]struct{}, edges map[string][]string) bool {
	state := make(map[string]uint8, len(nodes))
	var visit func(string) bool
	visit = func(node string) bool {
		if state[node] == 1 {
			return true
		}
		if state[node] == 2 {
			return false
		}
		state[node] = 1
		for _, next := range edges[node] {
			if visit(next) {
				return true
			}
		}
		state[node] = 2
		return false
	}
	for node := range nodes {
		if visit(node) {
			return true
		}
	}
	return false
}

func validateGraphSourceRef(ref ArtifactRef, changeID ChangeID, source string) error {
	if err := ref.Validate(); err != nil {
		return fmt.Errorf("ticket graph %s artifact reference: %w", source, err)
	}
	expectedKind, expectedSchema := "plan", "Plan.v1"
	if source == "draft" {
		expectedKind, expectedSchema = "ticket_draft", "keystone.ticket-draft.v1"
	}
	if ref.ChangeID != changeID || ref.Role != ArtifactRoleOutput || ref.Kind != expectedKind || ref.SchemaVersion != expectedSchema {
		return fmt.Errorf("%w: ticket graph %s artifact reference is not an output of the change", ErrInvalidRequest, source)
	}
	return nil
}
