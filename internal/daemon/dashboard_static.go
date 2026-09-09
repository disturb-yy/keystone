package daemon

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		writeError(w, http.StatusMethodNotAllowed, "invalid_request", "method is not allowed")
		return
	}
	s.mu.RLock()
	directory := s.options.DashboardDir
	s.mu.RUnlock()
	if directory == "" {
		directory = filepath.Join("dashboard", "dist")
	}
	directory, err := filepath.Abs(directory)
	if err != nil {
		writeError(w, http.StatusNotFound, "dashboard_unavailable", "dashboard assets are unavailable")
		return
	}
	indexPath := filepath.Join(directory, "index.html")
	if _, err := os.Stat(indexPath); err != nil {
		writeError(w, http.StatusNotFound, "dashboard_unavailable", "dashboard assets are unavailable")
		return
	}

	requested := filepath.Clean(filepath.FromSlash(strings.TrimPrefix(r.URL.Path, "/")))
	if requested == "." || requested == string(filepath.Separator) || strings.HasPrefix(requested, ".."+string(filepath.Separator)) || requested == ".." {
		requested = "index.html"
	}
	assetPath := filepath.Join(directory, requested)
	relative, err := filepath.Rel(directory, assetPath)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		writeError(w, http.StatusNotFound, "dashboard_unavailable", "dashboard asset is unavailable")
		return
	}
	if info, err := os.Stat(assetPath); err != nil || !info.Mode().IsRegular() {
		assetPath = indexPath
	}
	http.ServeFile(w, r, assetPath)
}
