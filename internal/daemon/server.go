// Package daemon 提供 Keystone Control Plane Daemon 的生命周期与 HTTP 边界。
package daemon

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/disturb-yy/keystone/contracts/controlplane"
	"github.com/disturb-yy/keystone/internal/infrastructure/id"
	"github.com/disturb-yy/keystone/internal/infrastructure/localstate"
	"github.com/disturb-yy/keystone/internal/infrastructure/migration"
)

const (
	databaseProbeTimeout = time.Second
	shutdownTimeout      = 2 * time.Second
	// stopResponseWindow 给客户端完成同实例 stop 重试留出响应后的异步窗口。
	stopResponseWindow = 100 * time.Millisecond
)

// Options 配置 Daemon 的启动行为。
type Options struct {
	// BootingGate 在 Daemon 已监听但尚未完成数据库启动时调用，供测试控制启动阶段。
	// 生产调用方保持 nil 即可。
	BootingGate func(context.Context) error
	// OnBooting 在 loopback listener 已绑定且 HTTP 服务已启动后调用，供测试发现端点。
	OnBooting func(*Server)
	// ShutdownTimeout 覆盖优雅关闭 HTTP Server 的等待时长；零值使用默认值。
	ShutdownTimeout time.Duration
}

// Server 是 Daemon 的运行句柄，并拥有本次运行的本机状态、HTTP 和数据库资源。
type Server struct {
	dataDir string
	options Options

	mu         sync.RWMutex
	started    bool
	paths      localstate.Paths
	lock       *localstate.InstanceLock
	db         *sql.DB
	listener   net.Listener
	httpServer *http.Server
	serveErr   chan error
	stopCh     chan struct{}
	stopOnce   sync.Once
	instanceID string
	endpoint   string
	startedAt  string
	readiness  bool
}

// New 创建尚未运行的 Daemon 句柄。路径、目录、锁和监听器均在 Run 中按固定顺序创建。
func New(dataDir string, options Options) *Server {
	if options.ShutdownTimeout <= 0 {
		options.ShutdownTimeout = shutdownTimeout
	}
	return &Server{
		dataDir:  dataDir,
		options:  options,
		stopCh:   make(chan struct{}),
		serveErr: make(chan error, 1),
	}
}

// Run 启动 Daemon，并在收到 stop 请求、上下文取消或服务异常后完成资源清理。
func Run(ctx context.Context, dataDir string, options Options) error {
	return New(dataDir, options).Run(ctx)
}

// Addr 返回已绑定的 loopback endpoint；监听器尚未绑定时返回空字符串。
func (s *Server) Addr() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.endpoint
}

// Endpoint 返回已绑定的 loopback endpoint，是 Addr 的语义别名。
func (s *Server) Endpoint() string {
	return s.Addr()
}

// InstanceID 返回当前 DaemonInstanceID；实例尚未生成时返回空字符串。
func (s *Server) InstanceID() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.instanceID
}

// Run 启动当前 Daemon 句柄。
func (s *Server) Run(ctx context.Context) error {
	if ctx == nil {
		return errors.New("run daemon: nil context")
	}
	if err := s.markStarted(); err != nil {
		return err
	}

	paths, err := localstate.Resolve(s.dataDir)
	if err != nil {
		return fmt.Errorf("resolve daemon data root: %w", err)
	}
	s.setPaths(paths)
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("initialize daemon: %w", err)
	}
	if err := paths.Initialize(); err != nil {
		return s.startupError(fmt.Errorf("initialize daemon data root: %w", err))
	}

	lock, err := localstate.Acquire(paths)
	if err != nil {
		return s.startupError(fmt.Errorf("acquire daemon instance lock: %w", err))
	}
	s.setLock(lock)
	if err := ctx.Err(); err != nil {
		return s.startupError(fmt.Errorf("acquire daemon instance lock: %w", err))
	}

	if err := s.bindHTTP(); err != nil {
		return s.startupError(err)
	}
	if err := s.runBootingGate(ctx); err != nil {
		return s.startupError(err)
	}
	if err := s.openAndMigrate(ctx); err != nil {
		return s.startupError(err)
	}
	if err := s.publishMetadata(); err != nil {
		return s.startupError(err)
	}
	s.setReadiness(true)

	return s.waitForStop(ctx)
}

func (s *Server) markStarted() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.started {
		return errors.New("run daemon: server already started")
	}
	s.started = true
	return nil
}

func (s *Server) setPaths(paths localstate.Paths) {
	s.mu.Lock()
	s.paths = paths
	s.mu.Unlock()
}

func (s *Server) setLock(lock *localstate.InstanceLock) {
	s.mu.Lock()
	s.lock = lock
	s.instanceID = id.New()
	s.startedAt = time.Now().UTC().Format(time.RFC3339Nano)
	s.mu.Unlock()
}

func (s *Server) bindHTTP() error {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("listen daemon endpoint: %w", err)
	}
	server := &http.Server{
		Handler: s.routes(),
		// 限制未发送请求头的连接，避免关闭时无限等待 StateNew 连接。
		ReadHeaderTimeout: time.Second,
	}
	s.mu.Lock()
	s.listener = listener
	s.httpServer = server
	s.endpoint = listener.Addr().String()
	s.mu.Unlock()
	go s.serveHTTP(server, listener)
	if s.options.OnBooting != nil {
		s.options.OnBooting(s)
	}
	return nil
}

func (s *Server) serveHTTP(server *http.Server, listener net.Listener) {
	err := server.Serve(listener)
	if errors.Is(err, http.ErrServerClosed) {
		err = nil
	}
	s.serveErr <- err
}

func (s *Server) runBootingGate(ctx context.Context) error {
	if s.options.BootingGate == nil {
		return nil
	}
	if err := s.options.BootingGate(ctx); err != nil {
		return fmt.Errorf("run daemon booting gate: %w", err)
	}
	return nil
}

func (s *Server) openAndMigrate(ctx context.Context) error {
	s.mu.RLock()
	paths := s.paths
	s.mu.RUnlock()
	db, err := sql.Open("sqlite", paths.DatabasePath)
	if err != nil {
		return fmt.Errorf("open daemon database: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if err := db.PingContext(ctx); err != nil {
		return errors.Join(fmt.Errorf("ping daemon database: %w", err), closeDatabase(db))
	}
	s.mu.Lock()
	s.db = db
	s.mu.Unlock()

	runner := migration.NewRunner(migration.DefaultMigrations())
	if err := runner.Apply(ctx, db); err != nil {
		return fmt.Errorf("apply daemon migrations: %w", err)
	}
	if _, err := readMigrationVersion(ctx, db); err != nil {
		return fmt.Errorf("query daemon schema migration version: %w", err)
	}
	return nil
}

func readMigrationVersion(ctx context.Context, db *sql.DB) (int, error) {
	const query = `SELECT COALESCE(MAX(version), 0) FROM t_schema_migrations`
	var version int
	if err := db.QueryRowContext(ctx, query).Scan(&version); err != nil {
		return 0, err
	}
	return version, nil
}

func (s *Server) publishMetadata() error {
	s.mu.RLock()
	paths, instanceID, endpoint, startedAt := s.paths, s.instanceID, s.endpoint, s.startedAt
	s.mu.RUnlock()
	metadata := localstate.Metadata{
		PID:        os.Getpid(),
		Endpoint:   endpoint,
		InstanceID: instanceID,
		StartedAt:  startedAt,
	}
	if err := localstate.PublishMetadata(paths, metadata); err != nil {
		return fmt.Errorf("publish daemon metadata: %w", err)
	}
	return nil
}

func (s *Server) setReadiness(ready bool) {
	s.mu.Lock()
	s.readiness = ready
	s.mu.Unlock()
}

func (s *Server) startupError(err error) error {
	return errors.Join(err, s.shutdownResources())
}

func (s *Server) waitForStop(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return s.shutdownResources()
	case <-s.stopCh:
		return s.shutdownResources()
	case err := <-s.serveErr:
		if err == nil {
			return s.shutdownResources()
		}
		return errors.Join(fmt.Errorf("serve daemon HTTP: %w", err), s.shutdownResources())
	}
}

func (s *Server) shutdownResources() error {
	s.mu.Lock()
	server := s.httpServer
	db := s.db
	paths := s.paths
	instanceID := s.instanceID
	lock := s.lock
	s.httpServer = nil
	s.listener = nil
	s.db = nil
	s.lock = nil
	s.readiness = false
	s.mu.Unlock()

	var shutdownErr error
	if server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), s.options.ShutdownTimeout)
		shutdownErr = server.Shutdown(ctx)
		cancel()
	}
	closeErr := closeDatabase(db)
	metadataErr := error(nil)
	if instanceID != "" {
		metadataErr = localstate.ClearMetadata(paths, instanceID)
	}
	lockErr := error(nil)
	if lock != nil {
		lockErr = lock.Release()
	}
	return errors.Join(
		wrapShutdownError("shutdown daemon HTTP server", shutdownErr),
		wrapShutdownError("close daemon database", closeErr),
		wrapShutdownError("clear daemon metadata", metadataErr),
		wrapShutdownError("release daemon instance lock", lockErr),
	)
}

func closeDatabase(db *sql.DB) error {
	if db == nil {
		return nil
	}
	return db.Close()
}

func wrapShutdownError(operation string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", operation, err)
}

func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.handleHealth)
	mux.HandleFunc("/v1/daemon/status", s.handleStatus)
	mux.HandleFunc("/v1/daemon/stop", s.handleStop)
	return mux
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "invalid_request", "method is not allowed")
		return
	}
	ready := s.databaseReady(r.Context())
	status := http.StatusServiceUnavailable
	if ready {
		status = http.StatusOK
	}
	writeJSON(w, status, controlplane.HealthResponse{Ready: ready})
}

func (s *Server) databaseReady(ctx context.Context) bool {
	s.mu.RLock()
	ready, db := s.readiness, s.db
	s.mu.RUnlock()
	if !ready || db == nil {
		return false
	}
	probeCtx, cancel := context.WithTimeout(ctx, databaseProbeTimeout)
	defer cancel()
	if err := db.PingContext(probeCtx); err != nil {
		s.markDatabaseUnavailable(ctx)
		return false
	}
	return true
}

func (s *Server) markDatabaseUnavailable(ctx context.Context) {
	if ctx != nil && ctx.Err() != nil {
		return
	}
	s.mu.Lock()
	s.readiness = false
	s.mu.Unlock()
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "invalid_request", "method is not allowed")
		return
	}
	response, err := s.statusResponse(r.Context())
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "daemon status is unavailable")
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func (s *Server) statusResponse(ctx context.Context) (controlplane.DaemonStatusResponse, error) {
	s.mu.RLock()
	ready, db, paths, instanceID := s.readiness, s.db, s.paths, s.instanceID
	s.mu.RUnlock()
	if !ready || db == nil {
		return controlplane.DaemonStatusResponse{}, errors.New("daemon status is not ready")
	}
	probeCtx, cancel := context.WithTimeout(ctx, databaseProbeTimeout)
	defer cancel()
	if err := db.PingContext(probeCtx); err != nil {
		s.markDatabaseUnavailable(ctx)
		return controlplane.DaemonStatusResponse{}, err
	}
	version, err := readMigrationVersion(probeCtx, db)
	if err != nil {
		s.markDatabaseUnavailable(ctx)
		return controlplane.DaemonStatusResponse{}, err
	}
	return controlplane.DaemonStatusResponse{
		DatabasePath:           paths.DatabasePath,
		SchemaMigrationVersion: version,
		DaemonReadiness:        true,
		DaemonInstanceID:       instanceID,
	}, nil
}

func (s *Server) handleStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "invalid_request", "method is not allowed")
		return
	}
	request, err := decodeStopRequest(r)
	if err != nil || strings.TrimSpace(request.DaemonInstanceID) == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "daemon_instance_id is required")
		return
	}
	if !s.acceptStop(request.DaemonInstanceID) {
		writeError(w, http.StatusConflict, "instance_mismatch", "daemon instance id does not match")
		return
	}
	s.disableHTTPKeepAlives()
	writeJSON(w, http.StatusOK, controlplane.DaemonStopResponse{
		Accepted:         true,
		DaemonInstanceID: s.InstanceID(),
	})
	s.scheduleStop()
}

func (s *Server) disableHTTPKeepAlives() {
	s.mu.RLock()
	server := s.httpServer
	s.mu.RUnlock()
	if server != nil {
		server.SetKeepAlivesEnabled(false)
	}
}

func decodeStopRequest(r *http.Request) (controlplane.DaemonStopRequest, error) {
	defer r.Body.Close()
	decoder := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	var request controlplane.DaemonStopRequest
	if err := decoder.Decode(&request); err != nil {
		return controlplane.DaemonStopRequest{}, err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return controlplane.DaemonStopRequest{}, errors.New("multiple JSON values")
		}
		return controlplane.DaemonStopRequest{}, err
	}
	return request, nil
}

func (s *Server) acceptStop(instanceID string) bool {
	s.mu.Lock()
	match := instanceID == s.instanceID
	s.mu.Unlock()
	return match
}

func (s *Server) scheduleStop() {
	s.stopOnce.Do(func() {
		timer := time.NewTimer(stopResponseWindow)
		go func() {
			defer timer.Stop()
			<-timer.C
			close(s.stopCh)
		}()
	})
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, controlplane.ErrorEnvelope{Code: code, Message: message})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		return
	}
}
