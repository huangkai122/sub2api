//go:build unit

package service

import (
	"context"
	"strings"
	"testing"
)

// TestRunCheckForModel_QwenMimoArk_DefaultPaths 验证三个新 provider 走 OpenAI 兼容协议：
// Bearer 鉴权 + chat body + choices.0.message.content 抽取，且各自命中官方默认路径。
func TestRunCheckForModel_QwenMimoArk_DefaultPaths(t *testing.T) {
	cases := []struct {
		provider string
		wantPath string
	}{
		{MonitorProviderQwen, providerQwenPath},
		{MonitorProviderMimo, providerMimoPath},
		{MonitorProviderArk, providerArkPath},
	}
	for _, tc := range cases {
		t.Run(tc.provider, func(t *testing.T) {
			h := &openAICaptureHandler{}
			endpoint := setupFakeOpenAI(t, h)

			res := runCheckForModel(context.Background(), tc.provider, endpoint, "sk-test", "test-model", nil)

			if res.Status != MonitorStatusOperational {
				t.Fatalf("provider %s should pass challenge, got status=%s message=%q", tc.provider, res.Status, res.Message)
			}
			if h.lastPath != tc.wantPath {
				t.Errorf("provider %s expected default path %q, got %q", tc.provider, tc.wantPath, h.lastPath)
			}
			if h.lastBody["model"] != "test-model" {
				t.Errorf("body should contain model=test-model, got %v", h.lastBody["model"])
			}
			if _, ok := h.lastBody["messages"]; !ok {
				t.Error("body should contain OpenAI-compatible messages")
			}
			if h.lastHeaders.Get("Authorization") != "Bearer sk-test" {
				t.Errorf("expected bearer auth header, got %q", h.lastHeaders.Get("Authorization"))
			}
		})
	}
}

// TestRunCheckForModel_EndpointWithBasePath 验证 endpoint 自带 base path 时只追加协议后缀，
// 与账号 base_url 语义一致（token-plan、网关子路径场景）。
func TestRunCheckForModel_EndpointWithBasePath(t *testing.T) {
	h := &openAICaptureHandler{}
	endpoint := setupFakeOpenAI(t, h) + "/api/plan/v3"

	res := runCheckForModel(context.Background(), MonitorProviderArk, endpoint, "sk-ark", "ark-model", nil)

	if res.Status != MonitorStatusOperational {
		t.Fatalf("base-path endpoint should pass challenge, got status=%s message=%q", res.Status, res.Message)
	}
	if h.lastPath != "/api/plan/v3/chat/completions" {
		t.Errorf("expected suffix appended to base path, got %q", h.lastPath)
	}
}

// TestResolveRequestURL 纯函数覆盖全部拼接分支。
func TestResolveRequestURL(t *testing.T) {
	chatAdapter := providerAdapters[MonitorProviderQwen]
	cases := []struct {
		name     string
		endpoint string
		adapter  providerAdapter
		model    string
		want     string
	}{
		{
			name:     "origin only uses default path",
			endpoint: "https://dashscope.aliyuncs.com",
			adapter:  chatAdapter,
			model:    "qwen-plus",
			want:     "https://dashscope.aliyuncs.com/compatible-mode/v1/chat/completions",
		},
		{
			name:     "origin with trailing slash",
			endpoint: "https://token-plan-cn.xiaomimimo.com/",
			adapter:  providerAdapters[MonitorProviderMimo],
			model:    "mimo-v2",
			want:     "https://token-plan-cn.xiaomimimo.com/v1/chat/completions",
		},
		{
			name:     "base path gets suffix only",
			endpoint: "https://ark.cn-beijing.volces.com/api/plan/v3",
			adapter:  providerAdapters[MonitorProviderArk],
			model:    "doubao-x",
			want:     "https://ark.cn-beijing.volces.com/api/plan/v3/chat/completions",
		},
		{
			name:     "base path trailing slash",
			endpoint: "https://relay.example.com/v1/",
			adapter:  chatAdapter,
			model:    "qwen-plus",
			want:     "https://relay.example.com/v1/chat/completions",
		},
		{
			name:     "full request url kept as-is",
			endpoint: "https://relay.example.com/v1/chat/completions",
			adapter:  chatAdapter,
			model:    "qwen-plus",
			want:     "https://relay.example.com/v1/chat/completions",
		},
		{
			name:     "gemini default path keeps model placeholder",
			endpoint: "https://generativelanguage.googleapis.com",
			adapter:  providerAdapters[MonitorProviderGemini],
			model:    "gemini-2.5-flash",
			want:     "https://generativelanguage.googleapis.com/v1beta/models/gemini-2.5-flash:generateContent",
		},
		{
			name:     "gemini base path appends model suffix",
			endpoint: "https://relay.example.com/gemini/v1beta",
			adapter:  providerAdapters[MonitorProviderGemini],
			model:    "gemini-2.5-flash",
			want:     "https://relay.example.com/gemini/v1beta/models/gemini-2.5-flash:generateContent",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := resolveRequestURL(tc.endpoint, tc.adapter, tc.model); got != tc.want {
				t.Errorf("resolveRequestURL(%q) = %q, want %q", tc.endpoint, got, tc.want)
			}
		})
	}
}

// TestValidateProvider_NewProviders 新 provider 注册后校验放行，未知值仍拒绝。
func TestValidateProvider_NewProviders(t *testing.T) {
	for _, p := range []string{MonitorProviderQwen, MonitorProviderMimo, MonitorProviderArk} {
		if err := validateProvider(p); err != nil {
			t.Errorf("validateProvider(%q) should pass, got %v", p, err)
		}
	}
	if err := validateProvider("not-a-provider"); err == nil {
		t.Error("unknown provider should be rejected")
	}
}

// TestValidateAPIMode_ResponsesRejectedForNewProviders responses 协议仍仅限 OpenAI。
func TestValidateAPIMode_ResponsesRejectedForNewProviders(t *testing.T) {
	for _, p := range []string{MonitorProviderQwen, MonitorProviderMimo, MonitorProviderArk} {
		if err := validateAPIMode(p, MonitorAPIModeResponses); err == nil {
			t.Errorf("responses mode should be rejected for provider %q", p)
		}
		if err := validateAPIMode(p, MonitorAPIModeChatCompletions); err != nil {
			t.Errorf("chat_completions should pass for provider %q, got %v", p, err)
		}
	}
}

// TestBuildRequestBody_MergeDenyListNewProviders 新 provider 的 merge 黑名单保护
// challenge 关键字段（model/messages/stream）不被用户覆盖。
func TestBuildRequestBody_MergeDenyListNewProviders(t *testing.T) {
	for _, p := range []string{MonitorProviderQwen, MonitorProviderMimo, MonitorProviderArk} {
		adapter := providerAdapters[p]
		body, err := buildRequestBody(adapter, p, MonitorAPIModeChatCompletions, "real-model", "prompt", &CheckOptions{
			BodyOverrideMode: MonitorBodyOverrideModeMerge,
			BodyOverride:     map[string]any{"model": "fake-model", "temperature": 0.5},
		})
		if err != nil {
			t.Fatalf("provider %s merge build failed: %v", p, err)
		}
		s := string(body)
		if strings.Contains(s, "fake-model") {
			t.Errorf("provider %s merge should drop denied key model, got %s", p, s)
		}
		if !strings.Contains(s, `"temperature":0.5`) {
			t.Errorf("provider %s merge should keep allowed key temperature, got %s", p, s)
		}
	}
}
