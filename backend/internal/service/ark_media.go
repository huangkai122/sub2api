package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/util/responseheaders"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

type ArkMediaEndpoint string

const (
	ArkMediaEndpointImagesGenerations ArkMediaEndpoint = "images_generations"
	ArkMediaEndpointVideosGenerations ArkMediaEndpoint = "videos_generations"
	ArkMediaEndpointVideoTaskStatus   ArkMediaEndpoint = "video_task_status"
)

func (e ArkMediaEndpoint) RequiresRequestBody() bool {
	return e != ArkMediaEndpointVideoTaskStatus
}

func (e ArkMediaEndpoint) IsGenerationRequest() bool {
	switch e {
	case ArkMediaEndpointImagesGenerations, ArkMediaEndpointVideosGenerations:
		return true
	default:
		return false
	}
}

func (e ArkMediaEndpoint) httpMethod() string {
	if e == ArkMediaEndpointVideoTaskStatus {
		return http.MethodGet
	}
	return http.MethodPost
}

func (e ArkMediaEndpoint) upstreamURL(baseURL, taskID string) (string, error) {
	baseURL = strings.TrimRight(baseURL, "/")
	switch e {
	case ArkMediaEndpointImagesGenerations:
		return baseURL + "/images/generations", nil
	case ArkMediaEndpointVideosGenerations:
		return baseURL + "/contents/generations/tasks", nil
	case ArkMediaEndpointVideoTaskStatus:
		if strings.TrimSpace(taskID) == "" {
			return "", fmt.Errorf("task_id is required for video task status")
		}
		return baseURL + "/contents/generations/tasks/" + taskID, nil
	default:
		return "", fmt.Errorf("unsupported ark media endpoint: %s", e)
	}
}

type ArkMediaRequestInfo struct {
	Model    string
	Prompt   string
	Text     string
	Duration int
	Size     string
}

func ParseArkMediaRequest(body []byte) ArkMediaRequestInfo {
	info := ArkMediaRequestInfo{}
	if !gjson.ValidBytes(body) {
		return info
	}
	info.Model = strings.TrimSpace(gjson.GetBytes(body, "model").String())
	info.Prompt = strings.TrimSpace(gjson.GetBytes(body, "prompt").String())
	info.Size = strings.TrimSpace(gjson.GetBytes(body, "size").String())
	if duration := gjson.GetBytes(body, "duration"); duration.Exists() && duration.Type == gjson.Number {
		info.Duration = int(duration.Int())
	}
	// For video generation, text prompt may be in content array
	if info.Prompt == "" {
		contentArr := gjson.GetBytes(body, "content")
		if contentArr.IsArray() {
			for _, item := range contentArr.Array() {
				if item.Get("type").String() == "text" {
					info.Text = strings.TrimSpace(item.Get("text").String())
					break
				}
			}
		}
	}
	return info
}

func (r ArkMediaRequestInfo) EffectivePrompt() string {
	if r.Prompt != "" {
		return r.Prompt
	}
	return r.Text
}

func (r ArkMediaRequestInfo) ModerationBody() []byte {
	prompt := r.EffectivePrompt()
	if prompt == "" {
		return nil
	}
	payload := map[string]string{"prompt": prompt}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil
	}
	return body
}

func ArkMediaVideoTaskSessionHash(taskID string) string {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return ""
	}
	return "ark-video:" + DeriveSessionHashFromSeed(taskID)
}

func (s *OpenAIGatewayService) BindArkMediaVideoTaskAccount(ctx context.Context, groupID *int64, taskID string, accountID int64) error {
	return s.BindStickySession(ctx, groupID, ArkMediaVideoTaskSessionHash(taskID), accountID)
}

func (s *OpenAIGatewayService) ForwardArkMedia(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	endpoint ArkMediaEndpoint,
	taskID string,
	body []byte,
) (*OpenAIForwardResult, error) {
	startTime := time.Now()
	if account == nil {
		return nil, fmt.Errorf("ark account is required")
	}
	if account.Platform != PlatformArk {
		return nil, fmt.Errorf("account platform %s is not supported for ark media", account.Platform)
	}

	token, _, err := s.GetAccessToken(ctx, account)
	if err != nil {
		return nil, err
	}
	targetURL, err := endpoint.upstreamURL(account.GetOpenAIBaseURL(), taskID)
	if err != nil {
		return nil, err
	}

	requestInfo := ParseArkMediaRequest(body)

	var bodyReader io.Reader
	if endpoint.RequiresRequestBody() {
		bodyReader = bytes.NewReader(body)
	}
	upstreamCtx, releaseUpstreamCtx := detachUpstreamContext(ctx)
	defer releaseUpstreamCtx()
	upstreamReq, err := http.NewRequestWithContext(upstreamCtx, endpoint.httpMethod(), targetURL, bodyReader)
	if err != nil {
		return nil, err
	}
	upstreamReq.Header.Set("Authorization", "Bearer "+token)
	upstreamReq.Header.Set("Accept", "application/json")
	if endpoint.RequiresRequestBody() {
		upstreamReq.Header.Set("Content-Type", "application/json")
	}

	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	upstreamStart := time.Now()
	resp, err := s.httpUpstream.Do(upstreamReq, proxyURL, account.ID, account.Concurrency)
	SetOpsLatencyMs(c, OpsUpstreamLatencyMsKey, time.Since(upstreamStart).Milliseconds())
	if err != nil {
		return nil, s.handleOpenAIUpstreamTransportError(ctx, c, account, err, false)
	}
	defer func() { _ = resp.Body.Close() }()

	requestIDHeader := firstNonEmpty(resp.Header.Get("x-request-id"), resp.Header.Get("x-tt-logid"))
	requestModel := requestInfo.Model
	if resp.StatusCode >= 400 {
		return s.handleArkMediaErrorResponse(ctx, resp, c, account, requestIDHeader, requestModel)
	}

	respBody, err := ReadUpstreamResponseBody(resp.Body, s.cfg, c, openAITooLargeError)
	if err != nil {
		return nil, err
	}
	writeArkMediaResponse(c, resp, respBody, s.responseHeaderFilter)
	usage := arkMediaUsageFromResponse(endpoint, requestInfo, respBody)
	return &OpenAIForwardResult{
		RequestID:            requestIDHeader,
		ResponseID:           usage.TaskID,
		Usage:                usage.Usage,
		Model:                requestModel,
		BillingModel:         requestModel,
		UpstreamModel:        requestModel,
		ResponseHeaders:      resp.Header.Clone(),
		Duration:             time.Since(startTime),
		ImageCount:           usage.ImageCount,
		ImageSize:            usage.ImageSize,
		VideoCount:           usage.VideoCount,
		VideoDurationSeconds: usage.VideoDurationSeconds,
	}, nil
}

type arkMediaUsageMetadata struct {
	TaskID               string
	Usage                OpenAIUsage
	ImageCount           int
	ImageSize            string
	VideoCount           int
	VideoDurationSeconds int
}

func arkMediaUsageFromResponse(endpoint ArkMediaEndpoint, requestInfo ArkMediaRequestInfo, responseBody []byte) arkMediaUsageMetadata {
	usage, _ := extractOpenAIUsageFromJSONBytes(responseBody)
	meta := arkMediaUsageMetadata{Usage: usage}
	switch endpoint {
	case ArkMediaEndpointImagesGenerations:
		imageCount := countOpenAIResponseImageOutputsFromJSONBytes(responseBody)
		if imageCount <= 0 {
			imageCount = 1
		}
		meta.ImageCount = imageCount
		meta.ImageSize = NormalizeImageBillingTierOrDefault(requestInfo.Size)
	case ArkMediaEndpointVideosGenerations:
		meta.TaskID = extractArkMediaVideoTaskID(responseBody)
		meta.VideoCount = 1
		meta.VideoDurationSeconds = NormalizeVideoBillingDurationSecondsOrDefault(requestInfo.Duration)
		meta.ImageCount = 1
	}
	return meta
}

func extractArkMediaVideoTaskID(body []byte) string {
	if len(body) == 0 || !gjson.ValidBytes(body) {
		return ""
	}
	for _, path := range []string{"id", "task_id", "data.id", "data.task_id"} {
		if id := strings.TrimSpace(gjson.GetBytes(body, path).String()); id != "" {
			return id
		}
	}
	return ""
}

func (s *OpenAIGatewayService) handleArkMediaErrorResponse(
	ctx context.Context,
	resp *http.Response,
	c *gin.Context,
	account *Account,
	requestIDHeader string,
	requestedModel string,
) (*OpenAIForwardResult, error) {
	body := s.readUpstreamErrorBody(resp)
	s.handleGrokAccountUpstreamError(ctx, account, resp.StatusCode, resp.Header, body)
	upstreamMsg := sanitizeUpstreamErrorMessage(strings.TrimSpace(extractUpstreamErrorMessage(body)))
	if upstreamMsg == "" {
		upstreamMsg = fmt.Sprintf("Ark upstream returned status %d", resp.StatusCode)
	}

	upstreamDetail := ""
	if s.cfg != nil && s.cfg.Gateway.LogUpstreamErrorBody {
		maxBytes := s.cfg.Gateway.LogUpstreamErrorBodyMaxBytes
		if maxBytes <= 0 {
			maxBytes = 2048
		}
		upstreamDetail = truncateString(string(body), maxBytes)
	}
	setOpsUpstreamError(c, resp.StatusCode, upstreamMsg, upstreamDetail)

	if status, errType, errMsg, matched := applyErrorPassthroughRule(
		c,
		account.Platform,
		resp.StatusCode,
		body,
		http.StatusBadGateway,
		"upstream_error",
		"Upstream request failed",
	); matched {
		MarkResponseCommitted(c)
		writeArkMediaErrorResponse(c, status, errType, errMsg)
		return nil, fmt.Errorf("upstream error: %d (passthrough rule matched) message=%s", resp.StatusCode, upstreamMsg)
	}

	if !account.ShouldHandleErrorCode(resp.StatusCode) {
		appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
			Platform:           account.Platform,
			AccountID:          account.ID,
			AccountName:        account.Name,
			UpstreamStatusCode: resp.StatusCode,
			UpstreamRequestID:  requestIDHeader,
			Kind:               "http_error",
			Message:            upstreamMsg,
			Detail:             upstreamDetail,
		})
		MarkResponseCommitted(c)
		writeArkMediaErrorResponse(c, http.StatusInternalServerError, "upstream_error", "Upstream gateway error")
		return nil, fmt.Errorf("upstream error: %d (not in custom error codes) message=%s", resp.StatusCode, upstreamMsg)
	}

	kind := "http_error"
	if s.shouldFailoverUpstreamError(resp.StatusCode) {
		kind = "failover"
	}
	appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
		Platform:           account.Platform,
		AccountID:          account.ID,
		AccountName:        account.Name,
		UpstreamStatusCode: resp.StatusCode,
		UpstreamRequestID:  requestIDHeader,
		Kind:               kind,
		Message:            upstreamMsg,
		Detail:             upstreamDetail,
	})
	if kind == "failover" {
		return nil, &UpstreamFailoverError{
			StatusCode:             resp.StatusCode,
			ResponseBody:           body,
			RetryableOnSameAccount: account.IsPoolMode() && account.IsPoolModeRetryableStatus(resp.StatusCode),
		}
	}

	MarkResponseCommitted(c)
	writeArkMediaErrorResponse(c, resp.StatusCode, arkMediaErrorType(resp.StatusCode), upstreamMsg)
	return nil, fmt.Errorf("upstream error: %d %s", resp.StatusCode, upstreamMsg)
}

func arkMediaErrorType(statusCode int) string {
	switch statusCode {
	case http.StatusBadRequest:
		return "invalid_request_error"
	case http.StatusNotFound:
		return "not_found_error"
	case http.StatusTooManyRequests:
		return "rate_limit_error"
	default:
		return "upstream_error"
	}
}

func writeArkMediaErrorResponse(c *gin.Context, statusCode int, errType, message string) {
	if c == nil || c.Writer == nil || c.Writer.Written() {
		return
	}
	c.JSON(statusCode, gin.H{
		"error": gin.H{
			"type":    strings.TrimSpace(errType),
			"message": strings.TrimSpace(message),
		},
	})
}

func writeArkMediaResponse(c *gin.Context, resp *http.Response, body []byte, filter *responseheaders.CompiledHeaderFilter) {
	if c == nil || resp == nil {
		return
	}
	writeOpenAIPassthroughResponseHeaders(c.Writer.Header(), resp.Header, filter)
	contentType := strings.TrimSpace(resp.Header.Get("Content-Type"))
	if contentType == "" {
		contentType = "application/json"
	}
	c.Data(resp.StatusCode, contentType, body)
}
