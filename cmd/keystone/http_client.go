package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/disturb-yy/keystone/contracts/controlplane"
)

const maxHTTPBodyBytes = 1 << 20

type daemonHTTPClient struct {
	httpClient HTTPDoer
	timeout    time.Duration
}

func (c *daemonHTTPClient) health(ctx context.Context, endpoint string) (controlplane.HealthResponse, error) {
	response, err := c.request(ctx, http.MethodGet, endpoint, "/healthz", nil)
	if err != nil {
		return controlplane.HealthResponse{}, err
	}
	if response.StatusCode != http.StatusOK && response.StatusCode != http.StatusServiceUnavailable {
		return controlplane.HealthResponse{}, c.protocolFailure(response, ErrorHealthNotReady, "健康检查 HTTP 状态异常")
	}
	var health controlplane.HealthResponse
	if err := decodeJSONResponse(response, &health); err != nil {
		return controlplane.HealthResponse{}, newCLIError(ErrorInvalidResponse, "健康检查 JSON 无效", err)
	}
	if response.StatusCode == http.StatusServiceUnavailable && health.Ready {
		return controlplane.HealthResponse{}, newCLIError(ErrorInvalidResponse, "503 健康响应必须为 ready=false", nil)
	}
	if response.StatusCode == http.StatusOK && !health.Ready {
		return health, newCLIError(ErrorHealthNotReady, "Daemon 尚未 ready", nil)
	}
	return health, nil
}

func (c *daemonHTTPClient) status(ctx context.Context, endpoint string) (controlplane.DaemonStatusResponse, error) {
	response, err := c.request(ctx, http.MethodGet, endpoint, "/v1/daemon/status", nil)
	if err != nil {
		return controlplane.DaemonStatusResponse{}, err
	}
	if response.StatusCode != http.StatusOK {
		return controlplane.DaemonStatusResponse{}, c.protocolFailure(response, ErrorStatusUnavailable, "Daemon status 不可用")
	}
	var status controlplane.DaemonStatusResponse
	if err := decodeJSONResponse(response, &status); err != nil {
		return controlplane.DaemonStatusResponse{}, newCLIError(ErrorInvalidResponse, "Daemon status JSON 无效", err)
	}
	return status, nil
}

func (c *daemonHTTPClient) stop(ctx context.Context, endpoint string, requestPayload controlplane.DaemonStopRequest) (controlplane.DaemonStopResponse, error) {
	response, err := c.request(ctx, http.MethodPost, endpoint, "/v1/daemon/stop", requestPayload)
	if err != nil {
		return controlplane.DaemonStopResponse{}, err
	}
	if response.StatusCode != http.StatusOK {
		return controlplane.DaemonStopResponse{}, c.protocolFailure(response, ErrorStatusUnavailable, "Daemon stop 请求失败")
	}
	var stopResponse controlplane.DaemonStopResponse
	if err := decodeJSONResponse(response, &stopResponse); err != nil {
		return controlplane.DaemonStopResponse{}, newCLIError(ErrorInvalidResponse, "Daemon stop JSON 无效", err)
	}
	return stopResponse, nil
}

func (c *daemonHTTPClient) init(ctx context.Context, endpoint, key string, payload controlplane.ProjectInitRequest) (controlplane.ProjectInitResponse, error) {
	response, err := c.requestWithHeaders(ctx, http.MethodPost, endpoint, "/v1/projects/init", payload, map[string]string{controlplane.IdempotencyKeyHeader: key})
	if err != nil {
		return controlplane.ProjectInitResponse{}, err
	}
	if response.StatusCode != http.StatusOK {
		return controlplane.ProjectInitResponse{}, c.protocolFailure(response, ErrorProjectInitFailed, "Project 初始化请求失败")
	}
	var result controlplane.ProjectInitResponse
	if err := decodeJSONResponse(response, &result); err != nil {
		return result, newCLIError(ErrorInvalidResponse, "Project 初始化 JSON 无效", err)
	}
	return result, nil
}

func (c *daemonHTTPClient) changeCreate(ctx context.Context, endpoint, key string, payload controlplane.ChangeCreateRequest) (controlplane.ChangeCreateResponse, error) {
	response, err := c.requestWithHeaders(ctx, http.MethodPost, endpoint, "/v1/changes", payload, map[string]string{controlplane.IdempotencyKeyHeader: key})
	if err != nil {
		return controlplane.ChangeCreateResponse{}, err
	}
	if response.StatusCode != http.StatusCreated {
		return controlplane.ChangeCreateResponse{}, c.protocolFailure(response, ErrorChangeFailed, "Change 创建请求失败")
	}
	var result controlplane.ChangeCreateResponse
	if err := decodeJSONResponse(response, &result); err != nil {
		return controlplane.ChangeCreateResponse{}, newCLIError(ErrorInvalidResponse, "Change 创建 JSON 无效", err)
	}
	return result, nil
}

func (c *daemonHTTPClient) changeList(ctx context.Context, endpoint, repositoryPath string) (controlplane.ChangeListResponse, error) {
	response, err := c.request(ctx, http.MethodGet, endpoint, "/v1/changes?repository_path="+url.QueryEscape(repositoryPath), nil)
	if err != nil {
		return controlplane.ChangeListResponse{}, err
	}
	if response.StatusCode != http.StatusOK {
		return controlplane.ChangeListResponse{}, c.protocolFailure(response, ErrorChangeFailed, "Change 列表请求失败")
	}
	var result controlplane.ChangeListResponse
	if err := decodeJSONResponse(response, &result); err != nil {
		return controlplane.ChangeListResponse{}, newCLIError(ErrorInvalidResponse, "Change 列表 JSON 无效", err)
	}
	return result, nil
}

func (c *daemonHTTPClient) changeShow(ctx context.Context, endpoint string, changeID string) (controlplane.ChangeCommandResponse, error) {
	response, err := c.request(ctx, http.MethodGet, endpoint, "/v1/changes/"+url.PathEscape(changeID), nil)
	if err != nil {
		return controlplane.ChangeCommandResponse{}, err
	}
	if response.StatusCode != http.StatusOK {
		return controlplane.ChangeCommandResponse{}, c.protocolFailure(response, ErrorChangeFailed, "Change 查询请求失败")
	}
	var result controlplane.ChangeCommandResponse
	if err := decodeJSONResponse(response, &result); err != nil {
		return controlplane.ChangeCommandResponse{}, newCLIError(ErrorInvalidResponse, "Change 查询 JSON 无效", err)
	}
	return result, nil
}

func (c *daemonHTTPClient) changeCommand(ctx context.Context, endpoint, key, changeID string, payload controlplane.ChangeCommandRequest) (controlplane.ChangeCommandResponse, error) {
	response, err := c.requestWithHeaders(ctx, http.MethodPost, endpoint, "/v1/changes/"+url.PathEscape(changeID)+"/commands", payload, map[string]string{controlplane.IdempotencyKeyHeader: key})
	if err != nil {
		return controlplane.ChangeCommandResponse{}, err
	}
	if response.StatusCode != http.StatusOK {
		return controlplane.ChangeCommandResponse{}, c.protocolFailure(response, ErrorChangeFailed, "Change 控制请求失败")
	}
	var result controlplane.ChangeCommandResponse
	if err := decodeJSONResponse(response, &result); err != nil {
		return controlplane.ChangeCommandResponse{}, newCLIError(ErrorInvalidResponse, "Change 控制 JSON 无效", err)
	}
	return result, nil
}

func (c *daemonHTTPClient) changeDecision(ctx context.Context, endpoint, key, changeID string, payload controlplane.HumanDecisionRequest) (controlplane.HumanDecisionResponse, error) {
	response, err := c.requestWithHeaders(ctx, http.MethodPost, endpoint, "/v1/changes/"+url.PathEscape(changeID)+"/decisions", payload, map[string]string{controlplane.IdempotencyKeyHeader: key})
	if err != nil {
		return controlplane.HumanDecisionResponse{}, err
	}
	if response.StatusCode != http.StatusOK {
		return controlplane.HumanDecisionResponse{}, c.protocolFailure(response, ErrorChangeFailed, "Change 决策请求失败")
	}
	var result controlplane.HumanDecisionResponse
	if err := decodeJSONResponse(response, &result); err != nil {
		return controlplane.HumanDecisionResponse{}, newCLIError(ErrorInvalidResponse, "Change 决策 JSON 无效", err)
	}
	return result, nil
}

func (c *daemonHTTPClient) request(ctx context.Context, method, endpoint, path string, payload any) (*http.Response, error) {
	return c.requestWithHeaders(ctx, method, endpoint, path, payload, nil)
}

func (c *daemonHTTPClient) requestWithHeaders(ctx context.Context, method, endpoint, path string, payload any, headers map[string]string) (*http.Response, error) {
	if err := validateDaemonEndpoint(endpoint); err != nil {
		return nil, newCLIError(ErrorMetadataInvalid, "Daemon endpoint 无效", err)
	}
	requestContext, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	body, err := encodeRequestPayload(payload)
	if err != nil {
		return nil, newCLIError(ErrorInvalidResponse, "编码 HTTP 请求 JSON 失败", err)
	}
	request, err := http.NewRequestWithContext(requestContext, method, "http://"+endpoint+path, body)
	if err != nil {
		return nil, newCLIError(ErrorInvalidResponse, "创建 Daemon HTTP 请求失败", err)
	}
	request.Header.Set("Accept", "application/json")
	for name, value := range headers {
		request.Header.Set(name, value)
	}
	if payload != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := c.httpClient.Do(request)
	if err != nil {
		return nil, newCLIError(ErrorEndpointUnreachable, "Daemon endpoint 不可达", err)
	}
	return response, nil
}

func encodeRequestPayload(payload any) (io.Reader, error) {
	if payload == nil {
		return nil, nil
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(encoded), nil
}

func (c *daemonHTTPClient) protocolFailure(response *http.Response, fallback ErrorCategory, message string) error {
	var envelope controlplane.ErrorEnvelope
	if err := decodeJSONResponse(response, &envelope); err != nil {
		return newCLIError(ErrorInvalidResponse, message+"，错误 envelope 无效", err)
	}
	if strings.TrimSpace(envelope.Code) == "" || strings.TrimSpace(envelope.Message) == "" {
		return newCLIError(ErrorInvalidResponse, message+"，错误 envelope 字段缺失", nil)
	}
	category := fallback
	if envelope.Code == string(ErrorInstanceMismatch) {
		category = ErrorInstanceMismatch
	}
	return newCLIError(category, envelope.Message, fmt.Errorf("daemon error code %s", envelope.Code))
}

func decodeJSONResponse(response *http.Response, target any) error {
	if response == nil {
		return errors.New("HTTP response is nil")
	}
	if response.Body == nil {
		return errors.New("HTTP response body is nil")
	}
	defer response.Body.Close()
	if err := requireJSONContentType(response.Header.Get("Content-Type")); err != nil {
		return err
	}
	decoder := json.NewDecoder(io.LimitReader(response.Body, maxHTTPBodyBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("HTTP response contains multiple JSON values")
		}
		return err
	}
	return nil
}

func requireJSONContentType(value string) error {
	mediaType, _, err := mime.ParseMediaType(value)
	if err != nil || !strings.EqualFold(mediaType, "application/json") {
		return fmt.Errorf("Content-Type must be application/json, got %q", value)
	}
	return nil
}
