package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/util/responseheaders"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

// QwenMediaEndpoint identifies Qwen (DashScope / Token Plan) async media APIs.
//
// Qwen image/video generation on the Token Plan endpoint uses the DashScope
// native async task protocol:
//   - submit: POST {origin}/api/v1/services/aigc/<service> with header
//     "X-DashScope-Async: enable", returning output.task_id
//   - query:  GET  {origin}/api/v1/tasks/{task_id}, returning output.task_status
//     (PENDING/RUNNING/SUCCEEDED/FAILED) and the result payload.
type QwenMediaEndpoint string

const (
	QwenMediaEndpointImagesGenerations QwenMediaEndpoint = "images_generations"
	QwenMediaEndpointVideosGenerations QwenMediaEndpoint = "videos_generations"
	QwenMediaEndpointTaskStatus        QwenMediaEndpoint = "task_status"
)

const (
	qwenMediaDefaultAPIOrigin = "https://token-plan.cn-beijing.maas.aliyuncs.com"
	qwenMediaImageSynthPath   = "/api/v1/services/aigc/multimodal-generation/generation"
	qwenMediaVideoSynthPath   = "/api/v1/services/aigc/video-generation/video-synthesis"
	qwenMediaTasksPath        = "/api/v1/tasks/"

	// Qwen image tasks usually finish within seconds; the gateway polls
	// server-side so downstream OpenAI-compatible clients get a synchronous
	// /v1/images/generations experience.
	qwenMediaImagePollInterval = 2 * time.Second
	qwenMediaImagePollTimeout  = 110 * time.Second
)

func (e QwenMediaEndpoint) RequiresRequestBody() bool {
	return e != QwenMediaEndpointTaskStatus
}

func (e QwenMediaEndpoint) IsGenerationRequest() bool {
	switch e {
	case QwenMediaEndpointImagesGenerations, QwenMediaEndpointVideosGenerations:
		return true
	default:
		return false
	}
}

func (e QwenMediaEndpoint) httpMethod() string {
	if e == QwenMediaEndpointTaskStatus {
		return http.MethodGet
	}
	return http.MethodPost
}

func (e QwenMediaEndpoint) upstreamURL(baseURL, taskID string) (string, error) {
	origin := qwenMediaAPIOrigin(baseURL)
	switch e {
	case QwenMediaEndpointImagesGenerations:
		return origin + qwenMediaImageSynthPath, nil
	case QwenMediaEndpointVideosGenerations:
		return origin + qwenMediaVideoSynthPath, nil
	case QwenMediaEndpointTaskStatus:
		if strings.TrimSpace(taskID) == "" {
			return "", fmt.Errorf("task_id is required for qwen media task status")
		}
		return origin + qwenMediaTasksPath + url.PathEscape(strings.TrimSpace(taskID)), nil
	default:
		return "", fmt.Errorf("unsupported qwen media endpoint: %s", e)
	}
}

// qwenMediaAPIOrigin extracts scheme://host from the account base URL.
// Account base_url usually carries the OpenAI-compatible base path
// (e.g. https://token-plan.cn-beijing.maas.aliyuncs.com/compatible-mode/v1 or
// https://dashscope.aliyuncs.com/compatible-mode/v1), while the DashScope
// native async APIs live on the origin root (/api/v1/...).
func qwenMediaAPIOrigin(baseURL string) string {
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" {
		return qwenMediaDefaultAPIOrigin
	}
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed == nil || parsed.Scheme == "" || parsed.Host == "" {
		return qwenMediaDefaultAPIOrigin
	}
	return parsed.Scheme + "://" + parsed.Host
}

// QwenMediaRequestInfo captures the fields needed for logging, billing and
// content moderation, extracted from either the flat OpenAI-ish request shape
// or the native DashScope task shape.
type QwenMediaRequestInfo struct {
	Model      string
	Prompt     string
	Size       string
	N          int
	Resolution string
	Ratio      string
	Duration   int
}

func ParseQwenMediaRequest(body []byte) QwenMediaRequestInfo {
	info := QwenMediaRequestInfo{}
	if !gjson.ValidBytes(body) {
		return info
	}
	get := func(paths ...string) gjson.Result {
		for _, p := range paths {
			if r := gjson.GetBytes(body, p); r.Exists() {
				return r
			}
		}
		return gjson.Result{}
	}
	info.Model = strings.TrimSpace(get("model").String())
	info.Prompt = strings.TrimSpace(get("prompt", "input.prompt").String())
	info.Size = strings.TrimSpace(get("size", "parameters.size").String())
	info.Resolution = strings.TrimSpace(get("resolution", "parameters.resolution").String())
	info.Ratio = strings.TrimSpace(get("ratio", "parameters.ratio").String())
	if n := get("n", "parameters.n"); n.Exists() && n.Type == gjson.Number {
		info.N = int(n.Int())
	}
	if d := get("duration", "parameters.duration"); d.Exists() && d.Type == gjson.Number {
		info.Duration = int(d.Int())
	}
	return info
}

func (r QwenMediaRequestInfo) EffectivePrompt() string {
	return r.Prompt
}

func (r QwenMediaRequestInfo) ModerationBody() []byte {
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

// buildQwenMediaTaskBody converts the flat OpenAI-ish gateway request into the
// DashScope async task shape {"model":..., "input":{...}, "parameters":{...}}.
// Requests that already carry a native "input" object are passed through
// untouched so DashScope-native clients keep full control.
func buildQwenMediaTaskBody(endpoint QwenMediaEndpoint, body []byte) ([]byte, error) {
	if len(body) == 0 || !gjson.ValidBytes(body) {
		return nil, fmt.Errorf("invalid request body")
	}
	if gjson.GetBytes(body, "input").IsObject() {
		return body, nil
	}
	root := gjson.ParseBytes(body)
	model := strings.TrimSpace(root.Get("model").String())
	if model == "" {
		return nil, fmt.Errorf("model is required")
	}

	input := map[string]any{}
	params := map[string]any{}
	copyString := func(dst map[string]any, key string) {
		if v := strings.TrimSpace(root.Get(key).String()); v != "" {
			dst[key] = v
		}
	}
	copyInt := func(dst map[string]any, key string) {
		if r := root.Get(key); r.Exists() && r.Type == gjson.Number {
			dst[key] = r.Int()
		}
	}
	copyBool := func(dst map[string]any, key string) {
		if r := root.Get(key); r.Exists() && (r.Type == gjson.True || r.Type == gjson.False) {
			dst[key] = r.Bool()
		}
	}

	// DashScope task input fields.
	for _, key := range []string{"prompt", "negative_prompt", "img_url", "ref_img"} {
		copyString(input, key)
	}

	// Extract prompt and media from OpenAI-style content array if present.
	contentArr := root.Get("content")
	if contentArr.IsArray() {
		var mediaItems []map[string]any
		for _, item := range contentArr.Array() {
			itemType := item.Get("type").String()
			switch itemType {
			case "text":
				if text := strings.TrimSpace(item.Get("text").String()); text != "" && input["prompt"] == nil {
					input["prompt"] = text
				}
			case "image_url":
				if url := strings.TrimSpace(item.Get("image_url.url").String()); url != "" {
					mediaItems = append(mediaItems, map[string]any{"type": "first_frame", "url": url})
				}
			case "video_url":
				if url := strings.TrimSpace(item.Get("video_url.url").String()); url != "" {
					mediaItems = append(mediaItems, map[string]any{"type": "video", "url": url})
				}
			}
		}
		if len(mediaItems) > 0 {
			input["media"] = mediaItems
		}
	}

	switch endpoint {
	case QwenMediaEndpointImagesGenerations:
		// Token Plan multimodal-generation uses messages format
		prompt := strings.TrimSpace(root.Get("prompt").String())
		if prompt == "" {
			// Try content array (OpenAI-style)
			contentArr := root.Get("content")
			if contentArr.IsArray() {
				for _, item := range contentArr.Array() {
					if item.Get("type").String() == "text" {
						prompt = strings.TrimSpace(item.Get("text").String())
						break
					}
				}
			}
		}
		if prompt != "" {
			input["messages"] = []map[string]any{
				{
					"role": "user",
					"content": []map[string]any{
						{"text": prompt},
					},
				},
			}
		}
		if size := normalizeQwenMediaImageSize(root.Get("size").String()); size != "" {
			params["size"] = size
		}
		copyInt(params, "n")
		copyInt(params, "seed")
		copyBool(params, "prompt_extend")
		copyBool(params, "watermark")
	case QwenMediaEndpointVideosGenerations:
		copyString(params, "resolution")
		copyString(params, "ratio")
		copyInt(params, "duration")
		copyInt(params, "seed")
		copyBool(params, "prompt_extend")
		copyBool(params, "watermark")
	}

	payload := map[string]any{
		"model": model,
		"input": input,
	}
	if len(params) > 0 {
		payload["parameters"] = params
	}
	return json.Marshal(payload)
}

// normalizeQwenMediaImageSize converts OpenAI-style "1024x1024" or aliases like
// "2K" to the DashScope "1024*1024" convention; native sizes pass through untouched.
func normalizeQwenMediaImageSize(size string) string {
	size = strings.TrimSpace(size)
	if size == "" {
		return "1024*1024"
	}
	// Map common aliases
	switch strings.ToLower(size) {
	case "1k", "1024x1024", "1024*1024":
		return "1024*1024"
	case "2k", "2048x2048", "2048*2048":
		return "2048*2048"
	case "4k", "4096x4096", "4096*4096":
		return "4096*4096"
	}
	if strings.Contains(size, "*") {
		return size
	}
	size = strings.ReplaceAll(size, "x", "*")
	return strings.ReplaceAll(size, "X", "*")
}

// QwenMediaTaskSessionHash derives a sticky-session hash from the upstream
// task id so status polls land on the same account that submitted the task.
func QwenMediaTaskSessionHash(taskID string) string {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return ""
	}
	return "qwen-media:" + DeriveSessionHashFromSeed(taskID)
}

func (s *OpenAIGatewayService) BindQwenMediaVideoTaskAccount(ctx context.Context, groupID *int64, taskID string, accountID int64) error {
	return s.BindStickySession(ctx, groupID, QwenMediaTaskSessionHash(taskID), accountID)
}

// extractQwenMediaTaskID pulls output.task_id (DashScope native) or a top-level
// task_id/id from an async task response.
func extractQwenMediaTaskID(body []byte) string {
	if len(body) == 0 || !gjson.ValidBytes(body) {
		return ""
	}
	for _, path := range []string{"output.task_id", "task_id", "id", "data.task_id", "data.id"} {
		if id := strings.TrimSpace(gjson.GetBytes(body, path).String()); id != "" {
			return id
		}
	}
	return ""
}

// ForwardQwenMedia forwards image/video generation requests to the Qwen
// DashScope async task API.
//
//   - videos_generations: submits the task and returns the upstream task
//     response immediately; clients poll GET /v1/videos/{task_id}.
//   - images_generations: submits the task, polls the task status server-side
//     and returns a synchronous OpenAI-style images response.
//   - task_status: proxies GET /api/v1/tasks/{task_id} and passes the response
//     through untouched.
func (s *OpenAIGatewayService) ForwardQwenMedia(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	endpoint QwenMediaEndpoint,
	taskID string,
	body []byte,
) (*OpenAIForwardResult, error) {
	startTime := time.Now()
	if account == nil {
		return nil, fmt.Errorf("qwen account is required")
	}
	if account.Platform != PlatformQwen {
		return nil, fmt.Errorf("account platform %s is not supported for qwen media", account.Platform)
	}

	token, _, err := s.GetAccessToken(ctx, account)
	if err != nil {
		return nil, err
	}

	upstreamCtx, releaseUpstreamCtx := detachUpstreamContext(ctx)
	defer releaseUpstreamCtx()

	requestInfo := ParseQwenMediaRequest(body)

	// Task status polling (both image and video tasks share /api/v1/tasks/{id}).
	if endpoint == QwenMediaEndpointTaskStatus {
		targetURL, err := endpoint.upstreamURL(account.GetOpenAIBaseURL(), taskID)
		if err != nil {
			return nil, err
		}
		respBody, resp, err := s.doQwenMediaRequest(upstreamCtx, c, account, endpoint.httpMethod(), token, targetURL, nil, false)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode >= 400 {
			return s.handleQwenMediaErrorResponse(ctx, resp, respBody, c, account, "", requestInfo.Model)
		}
		requestID := firstNonEmpty(resp.Header.Get("x-request-id"), strings.TrimSpace(gjson.GetBytes(respBody, "request_id").String()))
		writeQwenMediaResponse(c, resp, respBody, s.responseHeaderFilter)
		return &OpenAIForwardResult{
			RequestID:       requestID,
			Model:           requestInfo.Model,
			BillingModel:    requestInfo.Model,
			UpstreamModel:   requestInfo.Model,
			ResponseHeaders: resp.Header.Clone(),
			Duration:        time.Since(startTime),
		}, nil
	}

	targetURL, err := endpoint.upstreamURL(account.GetOpenAIBaseURL(), "")
	if err != nil {
		return nil, err
	}
	taskBody, err := buildQwenMediaTaskBody(endpoint, body)
	if err != nil {
		return nil, err
	}

	// Image generation on Token Plan is synchronous (no async header);
	// video generation uses async task protocol.
	useAsync := endpoint == QwenMediaEndpointVideosGenerations
	respBody, resp, err := s.doQwenMediaRequest(upstreamCtx, c, account, endpoint.httpMethod(), token, targetURL, taskBody, useAsync)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return s.handleQwenMediaErrorResponse(ctx, resp, respBody, c, account, "", requestInfo.Model)
	}

	submittedTaskID := extractQwenMediaTaskID(respBody)
	requestID := firstNonEmpty(resp.Header.Get("x-request-id"), strings.TrimSpace(gjson.GetBytes(respBody, "request_id").String()))

	if endpoint == QwenMediaEndpointVideosGenerations {
		// Async video: pass the task response through, client polls status.
		writeQwenMediaResponse(c, resp, respBody, s.responseHeaderFilter)
		return &OpenAIForwardResult{
			RequestID:            requestID,
			ResponseID:           submittedTaskID,
			Model:                requestInfo.Model,
			BillingModel:         requestInfo.Model,
			UpstreamModel:        requestInfo.Model,
			ResponseHeaders:      resp.Header.Clone(),
			Duration:             time.Since(startTime),
			VideoCount:           1,
			VideoDurationSeconds: NormalizeVideoBillingDurationSecondsOrDefault(requestInfo.Duration),
			VideoResolution:      NormalizeVideoBillingResolutionOrDefault(requestInfo.Resolution),
			ImageCount:           1,
		}, nil
	}

	// Synchronous image: the response already contains the result.
	// Try to extract images directly from the response (multimodal-generation format).
	imageData := extractQwenMediaImageURLs(respBody)
	if len(imageData) > 0 {
		// Synchronous response with images - convert to OpenAI format
		imageResponse, _ := json.Marshal(map[string]any{
			"created": time.Now().Unix(),
			"data":    imageData,
		})
		imageCount := len(imageData)
		c.Data(http.StatusOK, "application/json", imageResponse)
		return &OpenAIForwardResult{
			RequestID:       requestID,
			ResponseID:      submittedTaskID,
			Model:           requestInfo.Model,
			BillingModel:    requestInfo.Model,
			UpstreamModel:   requestInfo.Model,
			ResponseHeaders: resp.Header.Clone(),
			Duration:        time.Since(startTime),
			ImageCount:      imageCount,
			ImageSize:       NormalizeImageBillingTierOrDefault(requestInfo.Size),
		}, nil
	}

	// Fallback: if response contains a task_id, poll it (async mode fallback)
	if submittedTaskID == "" {
		writeQwenMediaResponse(c, resp, respBody, s.responseHeaderFilter)
		return nil, fmt.Errorf("qwen image response contains no images or task_id")
	}

	finalTaskBody, pollErr := s.pollQwenMediaTask(ctx, upstreamCtx, c, account, token, submittedTaskID)
	if pollErr != nil {
		return nil, pollErr
	}

	imageCount := 0
	imageResponse, err := qwenMediaImageResponseFromTask(finalTaskBody, time.Now())
	if err != nil {
		setOpsUpstreamError(c, http.StatusBadGateway, "Qwen image task finished without usable results", truncateString(string(finalTaskBody), 512))
		MarkResponseCommitted(c)
		writeQwenMediaErrorResponse(c, http.StatusBadGateway, "upstream_error", "Qwen image task finished without usable results")
		return nil, fmt.Errorf("qwen image task %s: %w", submittedTaskID, err)
	}
	imageCount = countQwenMediaImageResults(finalTaskBody)
	if imageCount <= 0 {
		imageCount = requestInfo.N
	}
	if imageCount <= 0 {
		imageCount = 1
	}
	c.Data(http.StatusOK, "application/json", imageResponse)

	return &OpenAIForwardResult{
		RequestID:       requestID,
		ResponseID:      submittedTaskID,
		Model:           requestInfo.Model,
		BillingModel:    requestInfo.Model,
		UpstreamModel:   requestInfo.Model,
		ResponseHeaders: resp.Header.Clone(),
		Duration:        time.Since(startTime),
		ImageCount:      imageCount,
		ImageSize:       NormalizeImageBillingTierOrDefault(requestInfo.Size),
	}, nil
}

// doQwenMediaRequest issues one upstream HTTP call and reads the response body.
func (s *OpenAIGatewayService) doQwenMediaRequest(
	upstreamCtx context.Context,
	c *gin.Context,
	account *Account,
	method, token, targetURL string,
	body []byte,
	async bool,
) ([]byte, *http.Response, error) {
	var bodyReader io.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	}
	upstreamReq, err := http.NewRequestWithContext(upstreamCtx, method, targetURL, bodyReader)
	if err != nil {
		return nil, nil, err
	}
	upstreamReq.Header.Set("Authorization", "Bearer "+token)
	upstreamReq.Header.Set("Accept", "application/json")
	if async {
		upstreamReq.Header.Set("X-DashScope-Async", "enable")
	}
	if body != nil {
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
		return nil, nil, s.handleOpenAIUpstreamTransportError(upstreamCtx, c, account, err, false)
	}
	respBody, err := ReadUpstreamResponseBody(resp.Body, s.cfg, c, openAITooLargeError)
	_ = resp.Body.Close()
	if err != nil {
		return nil, nil, err
	}
	return respBody, resp, nil
}

// pollQwenMediaTask polls GET /api/v1/tasks/{task_id} until the task leaves
// PENDING/RUNNING. requestCtx (client context) aborts the wait early when the
// downstream client disconnects; upstreamCtx drives the actual HTTP calls.
func (s *OpenAIGatewayService) pollQwenMediaTask(
	requestCtx context.Context,
	upstreamCtx context.Context,
	c *gin.Context,
	account *Account,
	token, taskID string,
) ([]byte, error) {
	deadline := time.Now().Add(qwenMediaImagePollTimeout)
	queryURL := qwenMediaAPIOrigin(account.GetOpenAIBaseURL()) + qwenMediaTasksPath + url.PathEscape(taskID)

	for {
		respBody, resp, err := s.doQwenMediaRequest(upstreamCtx, c, account, http.MethodGet, token, queryURL, nil, false)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode >= 400 {
			upstreamMsg := sanitizeUpstreamErrorMessage(strings.TrimSpace(extractUpstreamErrorMessage(respBody)))
			if upstreamMsg == "" {
				upstreamMsg = fmt.Sprintf("Qwen task query returned status %d", resp.StatusCode)
			}
			setOpsUpstreamError(c, resp.StatusCode, upstreamMsg, truncateString(string(respBody), 512))
			MarkResponseCommitted(c)
			writeQwenMediaErrorResponse(c, resp.StatusCode, qwenMediaErrorType(resp.StatusCode), upstreamMsg)
			return nil, fmt.Errorf("qwen task %s query failed: status %d", taskID, resp.StatusCode)
		}

		switch strings.ToUpper(strings.TrimSpace(gjson.GetBytes(respBody, "output.task_status").String())) {
		case "SUCCEEDED":
			return respBody, nil
		case "FAILED", "UNKNOWN", "CANCELED":
			code := strings.TrimSpace(gjson.GetBytes(respBody, "output.code").String())
			message := strings.TrimSpace(gjson.GetBytes(respBody, "output.message").String())
			upstreamMsg := strings.TrimSpace(strings.Join([]string{code, message}, ": "))
			if upstreamMsg == "" {
				upstreamMsg = "Qwen image generation task failed"
			}
			setOpsUpstreamError(c, http.StatusBadGateway, upstreamMsg, truncateString(string(respBody), 512))
			MarkResponseCommitted(c)
			writeQwenMediaErrorResponse(c, http.StatusBadGateway, "upstream_error", upstreamMsg)
			return nil, fmt.Errorf("qwen task %s failed: %s", taskID, upstreamMsg)
		}

		if time.Now().After(deadline) {
			timeoutMsg := fmt.Sprintf("Qwen image task %s did not finish within %s; query it via GET /v1/videos/%s", taskID, qwenMediaImagePollTimeout, taskID)
			setOpsUpstreamError(c, http.StatusGatewayTimeout, timeoutMsg, "")
			MarkResponseCommitted(c)
			writeQwenMediaErrorResponse(c, http.StatusGatewayTimeout, "upstream_error", timeoutMsg)
			return nil, fmt.Errorf("qwen task %s polling timeout", taskID)
		}

		select {
		case <-requestCtx.Done():
			return nil, requestCtx.Err()
		case <-time.After(qwenMediaImagePollInterval):
		}
	}
}

// qwenMediaImageResponseFromTask converts a SUCCEEDED DashScope image task into
// the OpenAI images response shape {"created":..., "data":[{"url":...}]}.
// Supports both the legacy text2image format (output.results[*].url) and the
// Token Plan multimodal-generation format (output.choices[*].message.content[*].image).
func qwenMediaImageResponseFromTask(taskBody []byte, now time.Time) ([]byte, error) {
	data := extractQwenMediaImageURLs(taskBody)
	if len(data) == 0 {
		return nil, fmt.Errorf("task output contains no image url")
	}
	payload := map[string]any{
		"created": now.Unix(),
		"data":    data,
	}
	return json.Marshal(payload)
}

// extractQwenMediaImageURLs extracts image URLs from either response format.
func extractQwenMediaImageURLs(taskBody []byte) []map[string]any {
	var data []map[string]any

	// Try multimodal-generation format: output.choices[*].message.content[*].image
	choices := gjson.GetBytes(taskBody, "output.choices")
	if choices.IsArray() {
		choices.ForEach(func(_, choice gjson.Result) bool {
			content := choice.Get("message.content")
			if content.IsArray() {
				content.ForEach(func(_, item gjson.Result) bool {
					if img := strings.TrimSpace(item.Get("image").String()); img != "" {
						data = append(data, map[string]any{"url": img})
					}
					return true
				})
			}
			return true
		})
	}

	// Fallback to legacy text2image format: output.results[*].url
	if len(data) == 0 {
		results := gjson.GetBytes(taskBody, "output.results")
		if results.IsArray() {
			for _, item := range results.Array() {
				u := strings.TrimSpace(item.Get("url").String())
				if u == "" {
					if b64 := strings.TrimSpace(item.Get("b64_json").String()); b64 != "" {
						data = append(data, map[string]any{"b64_json": b64})
					}
					continue
				}
				data = append(data, map[string]any{"url": u})
			}
		}
	}

	return data
}

func countQwenMediaImageResults(taskBody []byte) int {
	return len(extractQwenMediaImageURLs(taskBody))
}

func (s *OpenAIGatewayService) handleQwenMediaErrorResponse(
	ctx context.Context,
	resp *http.Response,
	body []byte,
	c *gin.Context,
	account *Account,
	requestIDHeader string,
	requestedModel string,
) (*OpenAIForwardResult, error) {
	// Qwen 401/403 should not trigger long cooldowns like Grok does;
	// they usually indicate model/endpoint mismatch rather than credential revocation.
	switch resp.StatusCode {
	case http.StatusUnauthorized:
		s.tempUnscheduleQwen(ctx, account, 5*time.Minute, "qwen credentials unauthorized")
	case http.StatusForbidden:
		s.tempUnscheduleQwen(ctx, account, 2*time.Minute, "qwen access forbidden")
	}
	upstreamMsg := sanitizeUpstreamErrorMessage(strings.TrimSpace(extractUpstreamErrorMessage(body)))
	if upstreamMsg == "" {
		upstreamMsg = fmt.Sprintf("Qwen upstream returned status %d", resp.StatusCode)
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
		writeQwenMediaErrorResponse(c, status, errType, errMsg)
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
		writeQwenMediaErrorResponse(c, http.StatusInternalServerError, "upstream_error", "Upstream gateway error")
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
	writeQwenMediaErrorResponse(c, resp.StatusCode, qwenMediaErrorType(resp.StatusCode), upstreamMsg)
	return nil, fmt.Errorf("upstream error: %d %s", resp.StatusCode, upstreamMsg)
}

func qwenMediaErrorType(statusCode int) string {
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

func writeQwenMediaErrorResponse(c *gin.Context, statusCode int, errType, message string) {
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

func writeQwenMediaResponse(c *gin.Context, resp *http.Response, body []byte, filter *responseheaders.CompiledHeaderFilter) {
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

func (s *OpenAIGatewayService) tempUnscheduleQwen(ctx context.Context, account *Account, cooldown time.Duration, reason string) {
	if s == nil || account == nil {
		return
	}
	until := time.Now().Add(cooldown)
	if account.TempUnschedulableUntil != nil && account.TempUnschedulableUntil.After(until) {
		until = *account.TempUnschedulableUntil
	}
	s.BlockAccountScheduling(account, until, reason)
	if s.accountRepo != nil {
		stateCtx, cancel := openAIAccountStateContext(ctx)
		defer cancel()
		_ = s.accountRepo.SetTempUnschedulable(stateCtx, account.ID, until, reason)
	}
}
