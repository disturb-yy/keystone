package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/disturb-yy/keystone/contracts/controlplane"
	"github.com/disturb-yy/keystone/internal/work/domain"
)

func TestInitCommandEnsuresDaemonAndPostsProjectRequest(t *testing.T) {
	paths := testPaths(t)
	instanceID := "daemon-init"
	projectID := string(domain.NewProjectID())
	var requestPath, requestKey string
	deps := testDependencies(paths, func(context.Context, string, ...string) (DaemonProcess, error) {
		t.Fatal("init should reuse healthy daemon")
		return nil, nil
	})
	deps.HTTPClient = httpDoerFunc(func(r *http.Request) (*http.Response, error) {
		var payload any
		switch r.URL.Path {
		case "/healthz":
			payload = controlplane.HealthResponse{Ready: true}
		case "/v1/daemon/status":
			payload = controlplane.DaemonStatusResponse{DatabasePath: paths.DatabasePath, SchemaMigrationVersion: 2, DaemonReadiness: true, DaemonInstanceID: instanceID}
		case "/v1/projects/init":
			if r.Method != http.MethodPost {
				return nil, fmt.Errorf("init method = %s, want POST", r.Method)
			}
			requestKey = r.Header.Get(controlplane.IdempotencyKeyHeader)
			var request controlplane.ProjectInitRequest
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				return nil, fmt.Errorf("decode init request: %w", err)
			}
			requestPath = request.RepositoryPath
			payload = controlplane.ProjectInitResponse{Project: controlplane.ProjectDTO{ProjectID: projectID, RepositoryRoot: request.RepositoryPath, ManifestPath: filepath.Join(request.RepositoryPath, ".keystone", "project.yaml"), CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)}}
		default:
			return nil, fmt.Errorf("unexpected path %q", r.URL.Path)
		}
		body, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(bytes.NewReader(body))}, nil
	})
	if err := publishTestMetadata(paths, "127.0.0.1:12345", instanceID); err != nil {
		t.Fatal(err)
	}
	output, err := executeCLI(t, deps, "init", "--data-dir", paths.Root)
	if err != nil {
		t.Fatalf("init command error = %v", err)
	}
	if requestKey == "" {
		t.Fatal("init command did not send Idempotency-Key")
	}
	if !filepath.IsAbs(requestPath) || strings.TrimSpace(requestPath) == "" {
		t.Fatalf("repository_path = %q, want non-empty absolute path", requestPath)
	}
	var response controlplane.ProjectInitResponse
	if err := json.Unmarshal([]byte(output), &response); err != nil {
		t.Fatalf("decode init output: %v", err)
	}
	if response.Project.ProjectID != projectID || response.Project.RepositoryRoot != requestPath {
		t.Fatalf("init output = %+v, want project %q and root %q", response.Project, projectID, requestPath)
	}
}
