package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/disturb-yy/keystone/contracts/controlplane"
	"github.com/disturb-yy/keystone/internal/work/domain"
)

func TestDaemonHTTPClientInitSendsAbsolutePathAndIdempotencyKey(t *testing.T) {
	wantKey := "cli-init-key"
	wantPath := "/absolute/repository"
	client := &daemonHTTPClient{httpClient: httpDoerFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/projects/init" {
			return nil, fmt.Errorf("request = %s %s, want POST /v1/projects/init", r.Method, r.URL.Path)
		}
		if got := r.Header.Get(controlplane.IdempotencyKeyHeader); got != wantKey {
			return nil, fmt.Errorf("Idempotency-Key = %q, want %q", got, wantKey)
		}
		var request controlplane.ProjectInitRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			return nil, fmt.Errorf("decode request: %w", err)
		}
		if request.RepositoryPath != wantPath {
			return nil, fmt.Errorf("repository_path = %q, want %q", request.RepositoryPath, wantPath)
		}
		body, err := json.Marshal(controlplane.ProjectInitResponse{Project: controlplane.ProjectDTO{
			ProjectID: string(domain.NewProjectID()), RepositoryRoot: wantPath, ManifestPath: wantPath + "/.keystone/project.yaml", CreatedAt: "2026-09-05T00:00:00Z",
		}})
		if err != nil {
			return nil, err
		}
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(bytes.NewReader(body))}, nil
	}), timeout: time.Second}
	response, err := client.init(context.Background(), "127.0.0.1:12345", wantKey, controlplane.ProjectInitRequest{RepositoryPath: wantPath})
	if err != nil {
		t.Fatal(err)
	}
	if response.Project.RepositoryRoot != wantPath || response.Project.ManifestPath == "" {
		t.Fatalf("response = %+v", response.Project)
	}
}

type httpDoerFunc func(*http.Request) (*http.Response, error)

func (f httpDoerFunc) Do(request *http.Request) (*http.Response, error) { return f(request) }
