//go:build unit

package service

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestQwenMediaAPIOrigin(t *testing.T) {
	cases := []struct {
		name    string
		baseURL string
		want    string
	}{
		{"empty falls back to token plan", "", "https://token-plan.cn-beijing.maas.aliyuncs.com"},
		{"token plan compatible mode path is stripped", "https://token-plan.cn-beijing.maas.aliyuncs.com/compatible-mode/v1", "https://token-plan.cn-beijing.maas.aliyuncs.com"},
		{"dashscope compatible mode path is stripped", "https://dashscope.aliyuncs.com/compatible-mode/v1", "https://dashscope.aliyuncs.com"},
		{"origin only", "https://dashscope.aliyuncs.com", "https://dashscope.aliyuncs.com"},
		{"invalid falls back", "://not-a-url", "https://token-plan.cn-beijing.maas.aliyuncs.com"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := qwenMediaAPIOrigin(tc.baseURL); got != tc.want {
				t.Fatalf("qwenMediaAPIOrigin(%q) = %q, want %q", tc.baseURL, got, tc.want)
			}
		})
	}
}

func TestQwenMediaEndpointUpstreamURL(t *testing.T) {
	base := "https://token-plan.cn-beijing.maas.aliyuncs.com/compatible-mode/v1"

	img, err := QwenMediaEndpointImagesGenerations.upstreamURL(base, "")
	if err != nil || img != "https://token-plan.cn-beijing.maas.aliyuncs.com/api/v1/services/aigc/multimodal-generation/generation" {
		t.Fatalf("images url = %q, err = %v", img, err)
	}

	vid, err := QwenMediaEndpointVideosGenerations.upstreamURL(base, "")
	if err != nil || vid != "https://token-plan.cn-beijing.maas.aliyuncs.com/api/v1/services/aigc/video-generation/video-synthesis" {
		t.Fatalf("videos url = %q, err = %v", vid, err)
	}

	task, err := QwenMediaEndpointTaskStatus.upstreamURL(base, "task-123")
	if err != nil || task != "https://token-plan.cn-beijing.maas.aliyuncs.com/api/v1/tasks/task-123" {
		t.Fatalf("task url = %q, err = %v", task, err)
	}

	if _, err := QwenMediaEndpointTaskStatus.upstreamURL(base, "  "); err == nil {
		t.Fatal("expected error for empty task id")
	}
}

func TestBuildQwenMediaTaskBodyImage(t *testing.T) {
	in := []byte(`{"model":"wanx2.1-t2i-turbo","prompt":"一只在草地上奔跑的柯基","negative_prompt":"模糊","size":"1024x1024","n":2,"seed":42,"watermark":false}`)
	out, err := buildQwenMediaTaskBody(QwenMediaEndpointImagesGenerations, in)
	if err != nil {
		t.Fatalf("build failed: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(out, &payload); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if payload["model"] != "wanx2.1-t2i-turbo" {
		t.Fatalf("model = %v", payload["model"])
	}
	input := payload["input"].(map[string]any)
	if input["prompt"] != "一只在草地上奔跑的柯基" || input["negative_prompt"] != "模糊" {
		t.Fatalf("input = %+v", input)
	}
	params := payload["parameters"].(map[string]any)
	if params["size"] != "1024*1024" {
		t.Fatalf("size should be converted to DashScope convention, got %v", params["size"])
	}
	if params["n"].(float64) != 2 || params["seed"].(float64) != 42 {
		t.Fatalf("params = %+v", params)
	}
	if _, ok := params["watermark"]; !ok {
		t.Fatalf("explicit false watermark should be preserved, params = %+v", params)
	}
}

func TestBuildQwenMediaTaskBodyVideo(t *testing.T) {
	in := []byte(`{"model":"happyhorse-1.1-t2v","prompt":"一只小狗在雪地奔跑","resolution":"720P","ratio":"16:9","duration":5}`)
	out, err := buildQwenMediaTaskBody(QwenMediaEndpointVideosGenerations, in)
	if err != nil {
		t.Fatalf("build failed: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(out, &payload); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if payload["model"] != "happyhorse-1.1-t2v" {
		t.Fatalf("model = %v", payload["model"])
	}
	input := payload["input"].(map[string]any)
	if input["prompt"] != "一只小狗在雪地奔跑" {
		t.Fatalf("input = %+v", input)
	}
	params := payload["parameters"].(map[string]any)
	if params["resolution"] != "720P" || params["ratio"] != "16:9" || params["duration"].(float64) != 5 {
		t.Fatalf("params = %+v", params)
	}
}

func TestBuildQwenMediaTaskBodyVideoImageToVideo(t *testing.T) {
	in := []byte(`{"model":"wanx2.1-i2v-turbo","prompt":"让画面动起来","img_url":"https://example.com/a.png","resolution":"720P"}`)
	out, err := buildQwenMediaTaskBody(QwenMediaEndpointVideosGenerations, in)
	if err != nil {
		t.Fatalf("build failed: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(out, &payload); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	input := payload["input"].(map[string]any)
	if input["img_url"] != "https://example.com/a.png" {
		t.Fatalf("img_url should live in input, got %+v", input)
	}
}

func TestBuildQwenMediaTaskBodyNativePassthrough(t *testing.T) {
	in := []byte(`{"model":"wanx2.1-t2i-turbo","input":{"prompt":"原生格式"},"parameters":{"size":"1024*1024"}}`)
	out, err := buildQwenMediaTaskBody(QwenMediaEndpointImagesGenerations, in)
	if err != nil {
		t.Fatalf("build failed: %v", err)
	}
	if string(out) != string(in) {
		t.Fatalf("native DashScope body should pass through untouched, got %s", out)
	}
}

func TestBuildQwenMediaTaskBodyRequiresModel(t *testing.T) {
	if _, err := buildQwenMediaTaskBody(QwenMediaEndpointImagesGenerations, []byte(`{"prompt":"no model"}`)); err == nil {
		t.Fatal("expected error when model is missing")
	}
	if _, err := buildQwenMediaTaskBody(QwenMediaEndpointImagesGenerations, []byte(`not-json`)); err == nil {
		t.Fatal("expected error for invalid json")
	}
}

func TestNormalizeQwenMediaImageSize(t *testing.T) {
	cases := map[string]string{
		// Empty defaults to 1K.
		"": "1024*1024",
		// Aliases.
		"1K":        "1024*1024",
		"2K":        "2048*2048",
		"4K":        "4096*4096",
		"1024x1024": "1024*1024",
		"1024X1024": "1024*1024",
		"2048x2048": "2048*2048",
		// Native DashScope sizes pass through untouched.
		"1024*1024": "1024*1024",
		"720*1280":  "720*1280",
	}
	for in, want := range cases {
		if got := normalizeQwenMediaImageSize(in); got != want {
			t.Fatalf("normalizeQwenMediaImageSize(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestExtractQwenMediaTaskID(t *testing.T) {
	body := []byte(`{"output":{"task_id":"abc-123","task_status":"PENDING"},"request_id":"req-1"}`)
	if got := extractQwenMediaTaskID(body); got != "abc-123" {
		t.Fatalf("task id = %q", got)
	}
	if got := extractQwenMediaTaskID([]byte(`{"task_id":"top"}`)); got != "top" {
		t.Fatalf("top-level task id = %q", got)
	}
	if got := extractQwenMediaTaskID([]byte(`{"output":{}}`)); got != "" {
		t.Fatalf("missing task id should be empty, got %q", got)
	}
	if got := extractQwenMediaTaskID(nil); got != "" {
		t.Fatalf("nil body should give empty task id, got %q", got)
	}
}

func TestQwenMediaImageResponseFromTask(t *testing.T) {
	taskBody := []byte(`{"output":{"task_id":"t1","task_status":"SUCCEEDED","results":[{"url":"https://img1"},{"url":"https://img2"}]},"request_id":"r1"}`)
	out, err := qwenMediaImageResponseFromTask(taskBody, time.Unix(1700000000, 0))
	if err != nil {
		t.Fatalf("convert failed: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(out, &payload); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if payload["created"].(float64) != 1700000000 {
		t.Fatalf("created = %v", payload["created"])
	}
	data := payload["data"].([]any)
	if len(data) != 2 || data[0].(map[string]any)["url"] != "https://img1" {
		t.Fatalf("data = %+v", data)
	}
	if got := countQwenMediaImageResults(taskBody); got != 2 {
		t.Fatalf("image count = %d", got)
	}
}

func TestQwenMediaImageResponseFromTaskNoResults(t *testing.T) {
	if _, err := qwenMediaImageResponseFromTask([]byte(`{"output":{"task_status":"SUCCEEDED"}}`), time.Now()); err == nil {
		t.Fatal("expected error when results missing")
	}
	if _, err := qwenMediaImageResponseFromTask([]byte(`{"output":{"results":[{}]}}`), time.Now()); err == nil {
		t.Fatal("expected error when results have no url")
	}
}

func TestParseQwenMediaRequest(t *testing.T) {
	flat := []byte(`{"model":"happyhorse-1.1-t2v","prompt":"p","resolution":"720P","ratio":"16:9","duration":5}`)
	info := ParseQwenMediaRequest(flat)
	if info.Model != "happyhorse-1.1-t2v" || info.Prompt != "p" || info.Resolution != "720P" || info.Ratio != "16:9" || info.Duration != 5 {
		t.Fatalf("flat parse = %+v", info)
	}

	native := []byte(`{"model":"wanx2.1-t2i-turbo","input":{"prompt":"np"},"parameters":{"size":"1024*1024","n":3}}`)
	info = ParseQwenMediaRequest(native)
	if info.Model != "wanx2.1-t2i-turbo" || info.Prompt != "np" || info.Size != "1024*1024" || info.N != 3 {
		t.Fatalf("native parse = %+v", info)
	}

	if got := ParseQwenMediaRequest([]byte(`invalid`)); got.Model != "" {
		t.Fatalf("invalid body should give empty info, got %+v", got)
	}
}

func TestQwenMediaTaskSessionHash(t *testing.T) {
	if got := QwenMediaTaskSessionHash(""); got != "" {
		t.Fatalf("empty task id should give empty hash, got %q", got)
	}
	got := QwenMediaTaskSessionHash("task-1")
	if !strings.HasPrefix(got, "qwen-media:") {
		t.Fatalf("hash should carry qwen-media prefix, got %q", got)
	}
	if QwenMediaTaskSessionHash("task-1") != QwenMediaTaskSessionHash("task-1") {
		t.Fatal("hash should be deterministic")
	}
}

func TestQwenMediaEndpointFlags(t *testing.T) {
	if QwenMediaEndpointTaskStatus.RequiresRequestBody() {
		t.Fatal("task status must not require body")
	}
	if !QwenMediaEndpointImagesGenerations.RequiresRequestBody() || !QwenMediaEndpointVideosGenerations.RequiresRequestBody() {
		t.Fatal("generation endpoints require body")
	}
	if !QwenMediaEndpointImagesGenerations.IsGenerationRequest() || !QwenMediaEndpointVideosGenerations.IsGenerationRequest() {
		t.Fatal("generation endpoints should be generation requests")
	}
	if QwenMediaEndpointTaskStatus.IsGenerationRequest() {
		t.Fatal("task status is not a generation request")
	}
}
