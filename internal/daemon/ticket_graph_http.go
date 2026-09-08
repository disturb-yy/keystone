package daemon

import (
	"github.com/disturb-yy/keystone/contracts/controlplane"
	"github.com/disturb-yy/keystone/internal/work/domain"
)

func ticketGraphReadModelDTO(graph domain.CanonicalTicketGraph) controlplane.TicketGraphReadModel {
	result := controlplane.TicketGraphReadModel{
		GraphID: string(graph.ID), ChangeID: string(graph.ChangeID), ProjectID: string(graph.ProjectID), BaseRevision: graph.BaseRevision,
		PlanArtifact: artifactRefDTO(graph.PlanArtifactRef), DraftArtifact: artifactRefDTO(graph.DraftArtifactRef), TicketizeAgentRunID: string(graph.TicketizeAgentRunID),
		GeneratorName: graph.GeneratorName, GeneratorVersion: graph.GeneratorVersion, CreatedAt: graph.CreatedAt.UTC().Format(timeRFC3339Nano),
		Tickets: make([]controlplane.CanonicalTicketDTO, 0, len(graph.Tickets)), Dependencies: make([]controlplane.TicketDependencyDTO, 0, len(graph.Dependencies)),
	}
	for _, ticket := range graph.Tickets {
		criteria := make([]string, 0, len(ticket.AcceptanceCriteria))
		for _, criterion := range ticket.AcceptanceCriteria {
			criteria = append(criteria, criterion.Text)
		}
		result.Tickets = append(result.Tickets, controlplane.CanonicalTicketDTO{TicketID: string(ticket.ID), Ordinal: ticket.Ordinal, Title: ticket.Title, Scope: ticket.Scope, AcceptanceCriteria: criteria})
	}
	for _, dependency := range graph.Dependencies {
		result.Dependencies = append(result.Dependencies, controlplane.TicketDependencyDTO{DependentTicketID: string(dependency.DependentTicketID), BlockerTicketID: string(dependency.BlockerTicketID)})
	}
	for _, ticketID := range graph.StructuralFrontier() {
		result.StructuralFrontier = append(result.StructuralFrontier, string(ticketID))
	}
	return result
}
