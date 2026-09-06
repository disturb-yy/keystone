package daemon

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	_ "modernc.org/sqlite"

	workercontract "github.com/disturb-yy/keystone/contracts/worker"
	"github.com/disturb-yy/keystone/internal/infrastructure/migration"
	"github.com/disturb-yy/keystone/internal/infrastructure/workstore"
)

func TestWorkerProtocolHandlerValidatesLoopbackAuthAndStrictJSON(t *testing.T) {
	db, err := sql.Open("sqlite", "file:worker-http-test?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(`PRAGMA foreign_keys = ON`); err != nil {
		t.Fatal(err)
	}
	if err := migration.NewRunner(append(migration.DefaultMigrations(), workstore.Migrations()...)).Apply(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	store, err := workstore.New(db)
	if err != nil {
		t.Fatal(err)
	}
	secret, err := workstore.NewWorkerSecret()
	if err != nil {
		t.Fatal(err)
	}
	workerID := "worker-http"
	if err := store.PrepareWorker(context.Background(), workerID, secret); err != nil {
		t.Fatal(err)
	}
	handler := NewWorkerProtocolHandler(store, nil)

	register := func(body, remote, bearer string) *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/worker/v1/register", bytes.NewBufferString(body))
		request.RemoteAddr = remote
		if bearer != "" {
			request.Header.Set("Authorization", "Bearer "+bearer)
		}
		handler.ServeHTTP(recorder, request)
		return recorder
	}

	invalid := register(`{"worker_id":"worker-http","protocol_version":"v1","capabilities":[],"unknown":true}`, "127.0.0.1:1234", secret)
	if invalid.Code != http.StatusBadRequest {
		t.Fatalf("unknown field status = %d, want 400", invalid.Code)
	}
	unauthorized := register(`{"worker_id":"worker-http","protocol_version":"v1","capabilities":[]}`, "127.0.0.1:1234", "wrong-secret")
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("wrong bearer status = %d, want 401", unauthorized.Code)
	}
	outside := register(`{"worker_id":"worker-http","protocol_version":"v1","capabilities":[]}`, "192.0.2.1:1234", secret)
	if outside.Code != http.StatusForbidden {
		t.Fatalf("non-loopback status = %d, want 403", outside.Code)
	}
	valid := register(`{"worker_id":"worker-http","protocol_version":"v1","capabilities":["runtime:codex"]}`, "127.0.0.1:1234", secret)
	if valid.Code != http.StatusOK || bytes.Contains(valid.Body.Bytes(), []byte(secret)) {
		t.Fatalf("register response = %d %s, secret was returned or request failed", valid.Code, valid.Body.String())
	}

	pullRecorder := httptest.NewRecorder()
	pullRequest := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/worker/v1/pull", bytes.NewBufferString(`{"worker_id":"worker-http"}`))
	pullRequest.RemoteAddr = "127.0.0.1:1234"
	pullRequest.Header.Set("Authorization", "Bearer "+secret)
	handler.ServeHTTP(pullRecorder, pullRequest)
	if pullRecorder.Code != http.StatusOK || pullRecorder.Body.String() != "{\"assignment\":null}\n" {
		t.Fatalf("empty pull response = %d %q", pullRecorder.Code, pullRecorder.Body.String())
	}

	var response workercontract.RegisterResponse
	if err := json.Unmarshal(valid.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.WorkerID != workerID || response.HeartbeatIntervalSecs != 5 || response.LeaseTTLSeconds != 30 {
		t.Fatalf("register response = %+v", response)
	}
}
