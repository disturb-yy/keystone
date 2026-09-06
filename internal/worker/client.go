// Package worker 提供独立 Worker 使用的主动 Worker Protocol Client。
package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	workercontract "github.com/disturb-yy/keystone/contracts/worker"
)

// ProtocolError 表示 Daemon 返回的稳定协议错误，不包含响应原文。
type ProtocolError struct {
	Status int
	Code   string
}

func (e *ProtocolError) Error() string {
	if e == nil {
		return "worker protocol error"
	}
	return fmt.Sprintf("worker protocol request failed: status=%d code=%s", e.Status, e.Code)
}

// Retryable 判断错误是否允许重发同一个请求/Report。
func (e *ProtocolError) Retryable() bool {
	return e != nil && (e.Status == http.StatusServiceUnavailable || e.Code == "unavailable")
}

// Client 是只在 Worker 进程内保存 Bearer secret 的 HTTP Client。
type Client struct {
	BaseURL    string
	Secret     string
	HTTPClient *http.Client
}

// NewClient 创建 Worker Protocol Client；endpoint 可以是 host:port 或带 scheme 的 URL。
func NewClient(endpoint, secret string, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{}
	}
	base := strings.TrimRight(endpoint, "/")
	if !strings.Contains(base, "://") {
		base = "http://" + base
	}
	return &Client{BaseURL: base, Secret: secret, HTTPClient: httpClient}
}

// Register 向 Daemon 注册当前 WorkerInstance。
func (c *Client) Register(ctx context.Context, request workercontract.Register) (workercontract.RegisterResponse, error) {
	var response workercontract.RegisterResponse
	err := c.do(ctx, http.MethodPost, "/worker/v1/register", request, &response)
	return response, err
}

// Heartbeat 发送当前 Worker 的可用性和可选 Lease 摘要。
func (c *Client) Heartbeat(ctx context.Context, request workercontract.Heartbeat) (workercontract.HeartbeatResponse, error) {
	var response workercontract.HeartbeatResponse
	err := c.do(ctx, http.MethodPost, "/worker/v1/heartbeat", request, &response)
	return response, err
}

// Pull 请求一个已授权 Assignment；无任务时 response.Assignment 为 nil。
func (c *Client) Pull(ctx context.Context, request workercontract.PullRequest) (workercontract.PullResponse, error) {
	var response workercontract.PullResponse
	err := c.do(ctx, http.MethodPost, "/worker/v1/pull", request, &response)
	return response, err
}

// Report 提交一次执行事实；调用方负责在 unavailable 时重发同一请求。
func (c *Client) Report(ctx context.Context, request workercontract.Report) (workercontract.ReportResponse, error) {
	var response workercontract.ReportResponse
	err := c.do(ctx, http.MethodPost, "/worker/v1/report", request, &response)
	return response, err
}

func (c *Client) do(ctx context.Context, method, route string, request, response any) error {
	if c == nil || c.HTTPClient == nil || strings.TrimSpace(c.BaseURL) == "" || c.Secret == "" {
		return errors.New("worker protocol client is not configured")
	}
	body, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("encode worker protocol request: %w", err)
	}
	httpRequest, err := http.NewRequestWithContext(ctx, method, c.BaseURL+route, strings.NewReader(string(body)))
	if err != nil {
		return fmt.Errorf("create worker protocol request: %w", err)
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set("Authorization", "Bearer "+c.Secret)
	httpResponse, err := c.HTTPClient.Do(httpRequest)
	if err != nil {
		return fmt.Errorf("call worker protocol: %w", err)
	}
	defer httpResponse.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(httpResponse.Body, workercontract.MaxBodyBytes+1))
	if err != nil {
		return fmt.Errorf("read worker protocol response: %w", err)
	}
	if httpResponse.StatusCode < http.StatusOK || httpResponse.StatusCode >= http.StatusMultipleChoices {
		var payload workercontract.ErrorResponse
		if decodeErr := workercontract.DecodeStrict(responseBody, &payload); decodeErr != nil {
			return &ProtocolError{Status: httpResponse.StatusCode, Code: "protocol_invalid"}
		}
		return &ProtocolError{Status: httpResponse.StatusCode, Code: payload.Code}
	}
	if err := workercontract.DecodeStrict(responseBody, response); err != nil {
		return fmt.Errorf("decode worker protocol response: %w", err)
	}
	return nil
}
