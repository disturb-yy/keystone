package work

import (
	"context"
	"fmt"

	"github.com/disturb-yy/keystone/internal/work/domain"
)

// TicketGraph 查询一个 Change 已经成功提交的 Canonical Ticket Graph。
func (s *ChangeService) TicketGraph(ctx context.Context, changeID domain.ChangeID) (domain.CanonicalTicketGraph, error) {
	if ctx == nil || changeID == "" {
		return domain.CanonicalTicketGraph{}, fmt.Errorf("find ticket graph: %w", domain.ErrInvalidRequest)
	}
	port, ok := s.state.(TicketizeStatePort)
	if !ok {
		return domain.CanonicalTicketGraph{}, fmt.Errorf("find ticket graph: %w", domain.ErrUnavailable)
	}
	return port.FindTicketGraph(ctx, changeID)
}
