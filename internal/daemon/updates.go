package daemon

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/disturb-yy/keystone/contracts/controlplane"
)

const refreshHintHeartbeat = 25 * time.Second

type refreshSubscription struct {
	filterProjectID string
	filterChangeID  string
	ch              chan controlplane.RefreshHint
}

type refreshHub struct {
	mu     sync.Mutex
	nextID uint64
	subs   map[uint64]*refreshSubscription
	closed bool
}

func newRefreshHub() *refreshHub {
	return &refreshHub{subs: make(map[uint64]*refreshSubscription)}
}

func (h *refreshHub) subscribe(projectID, changeID string) (uint64, <-chan controlplane.RefreshHint, func()) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		channel := make(chan controlplane.RefreshHint)
		close(channel)
		return 0, channel, func() {}
	}
	h.nextID++
	id := h.nextID
	subscription := &refreshSubscription{filterProjectID: projectID, filterChangeID: changeID, ch: make(chan controlplane.RefreshHint, 8)}
	h.subs[id] = subscription
	return id, subscription.ch, func() { h.unsubscribe(id) }
}

func (h *refreshHub) unsubscribe(id uint64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	subscription, ok := h.subs[id]
	if !ok {
		return
	}
	delete(h.subs, id)
	close(subscription.ch)
}

func (h *refreshHub) publish(hint controlplane.RefreshHint) {
	if !validRefreshHint(hint) {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return
	}
	for _, subscription := range h.subs {
		if subscription.filterProjectID != "" && subscription.filterProjectID != hint.ProjectID {
			continue
		}
		if subscription.filterChangeID != "" && subscription.filterChangeID != hint.ChangeID {
			continue
		}
		select {
		case subscription.ch <- hint:
		default:
			// SSE 不是可靠队列；慢客户端丢弃提示后会在下一次连接或刷新时重新 Query。
		}
	}
}

func (h *refreshHub) close() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return
	}
	h.closed = true
	for id, subscription := range h.subs {
		delete(h.subs, id)
		close(subscription.ch)
	}
}

func (s *Server) publishRefresh(hint controlplane.RefreshHint) {
	s.mu.RLock()
	hub := s.updates
	s.mu.RUnlock()
	if hub != nil {
		hub.publish(hint)
	}
}

func (s *Server) handleUpdates(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "invalid_request", "method is not allowed")
		return
	}
	projectID := strings.TrimSpace(r.URL.Query().Get("project_id"))
	changeID := strings.TrimSpace(r.URL.Query().Get("change_id"))
	if projectID != "" && controlplane.ValidateProjectID(projectID) != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "project_id is invalid")
		return
	}
	if changeID != "" && controlplane.ValidateChangeID(changeID) != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "change_id is invalid")
		return
	}
	s.mu.RLock()
	hub := s.updates
	stopCh := s.stopCh
	s.mu.RUnlock()
	if hub == nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "update stream is unavailable")
		return
	}
	_, updates, unsubscribe := hub.subscribe(projectID, changeID)
	defer unsubscribe()
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "unavailable", "update stream does not support flushing")
		return
	}
	_, _ = fmt.Fprint(w, ": connected\n\n")
	flusher.Flush()
	heartbeat := time.NewTicker(refreshHintHeartbeat)
	defer heartbeat.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-stopCh:
			return
		case hint, ok := <-updates:
			if !ok {
				return
			}
			payload, err := json.Marshal(hint)
			if err != nil {
				return
			}
			if _, err := fmt.Fprintf(w, "event: refresh\ndata: %s\n\n", payload); err != nil {
				return
			}
			flusher.Flush()
		case <-heartbeat.C:
			if _, err := fmt.Fprint(w, ": keep-alive\n\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

func validRefreshHint(hint controlplane.RefreshHint) bool {
	if hint.ResourceType != "project" && hint.ResourceType != "change" && hint.ResourceType != "daemon" {
		return false
	}
	if hint.ProjectID != "" && controlplane.ValidateProjectID(hint.ProjectID) != nil {
		return false
	}
	if hint.ChangeID != "" && controlplane.ValidateChangeID(hint.ChangeID) != nil {
		return false
	}
	return hint.ResourceType != "change" || hint.ChangeID != ""
}
