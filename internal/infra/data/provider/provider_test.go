package provider

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"runtime"
	"sync"
	"testing"

	"devops-backend/internal/biz"
	"devops-backend/internal/conf"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

// funcName 获取函数名用于断言
func funcName(f createFunc) string {
	return runtime.FuncForPC(reflect.ValueOf(f).Pointer()).Name()
}

// --- 路由测试 ---

func TestResolve_ModelRouting(t *testing.T) {
	m := NewMixedProvider(conf.Eino{
		Clients: map[string]conf.Client{
			"openai":   {BaseURL: "https://api.openai.com/v1", APIKey: "sk-test"},
			"claude":   {BaseURL: "https://api.anthropic.com", APIKey: "sk-ant"},
			"deepseek": {BaseURL: "https://api.deepseek.com", APIKey: "sk-ds"},
			"gemini":   {APIKey: "gemini-key"},
			"grok":     {BaseURL: "https://api.x.ai/v1", APIKey: "xai-key"},
		},
	})

	tests := []struct {
		model    string
		wantFunc createFunc
	}{
		{"gpt-4o", newOpenAI},
		{"o3-mini", newOpenAI},
		{"claude-sonnet-4-5-20250929", newClaude},
		{"deepseek-r1", newDeepSeek},
		{"gemini-2.5-pro", newGemini},
		{"grok-3", newOpenAICompatible},
		{"llama-4-maverick", newOpenAI},
		{"unknown-model", newOpenAICompatible}, // fallback
	}

	for _, tt := range tests {
		fn, _ := m.resolve(tt.model, false)
		if funcName(fn) != funcName(tt.wantFunc) {
			t.Errorf("resolve(%q) = %s, want %s", tt.model, funcName(fn), funcName(tt.wantFunc))
		}
	}
}

func TestResolve_PrefixPriority(t *testing.T) {
	m := NewMixedProvider(conf.Eino{
		Clients: map[string]conf.Client{
			"openai":     {BaseURL: "https://api.openai.com/v1", APIKey: "sk-test"},
			"openrouter": {BaseURL: "https://openrouter.ai/api/v1", APIKey: "or-key"},
		},
	})

	tests := []struct {
		model    string
		wantFunc createFunc
	}{
		{"openrouter/openai/gpt-4o", newOpenRouter},
		{"openrouter/google/gemini-2.5-pro", newOpenRouter},
		{"openrouter/anthropic/claude-3.5-sonnet", newOpenRouter},
		{"gpt-4o", newOpenAI},           // 不带前缀走 openai
		{"bot-12345", newArkBot},         // bot- 前缀
		{"ep-20240101-abcde", newArk},    // ep- 前缀
	}

	for _, tt := range tests {
		fn, _ := m.resolve(tt.model, false)
		if funcName(fn) != funcName(tt.wantFunc) {
			t.Errorf("resolve(%q) = %s, want %s", tt.model, funcName(fn), funcName(tt.wantFunc))
		}
	}
}

func TestResolve_Override(t *testing.T) {
	m := NewMixedProvider(conf.Eino{
		Clients: map[string]conf.Client{
			"openai": {BaseURL: "https://api.openai.com/v1", APIKey: "sk-test"},
			"claude": {BaseURL: "https://api.anthropic.com", APIKey: "sk-ant"},
		},
		ModelOverrides: map[string]string{
			"my-claude-proxy": "openai", // 强制走 openai
		},
	})

	fn, cfg := m.resolve("my-claude-proxy", false)
	if funcName(fn) != funcName(newOpenAI) {
		t.Errorf("override: got %s, want newOpenAI", funcName(fn))
	}
	if cfg.APIKey != "sk-test" {
		t.Errorf("override config: got APIKey=%q, want sk-test", cfg.APIKey)
	}
}

func TestResolve_RawVsAdapted(t *testing.T) {
	m := NewMixedProvider(conf.Eino{
		Clients: map[string]conf.Client{
			"openai": {BaseURL: "https://api.openai.com/v1", APIKey: "sk-test"},
		},
	})

	adapted, _ := m.resolve("gpt-4o", false)
	raw, _ := m.resolve("gpt-4o", true)
	if funcName(adapted) == funcName(raw) {
		t.Errorf("adapted and raw should be different functions, both are %s", funcName(adapted))
	}
	if funcName(adapted) != funcName(newOpenAI) {
		t.Errorf("adapted: got %s, want newOpenAI", funcName(adapted))
	}
	if funcName(raw) != funcName(newOpenAIRaw) {
		t.Errorf("raw: got %s, want newOpenAIRaw", funcName(raw))
	}
}

func TestResolve_FallbackToOpenAIConfig(t *testing.T) {
	m := NewMixedProvider(conf.Eino{
		Clients: map[string]conf.Client{
			"openai": {BaseURL: "https://api.openai.com/v1", APIKey: "sk-test"},
		},
	})

	_, cfg := m.resolve("claude-sonnet-4-5-20250929", false)
	if cfg.APIKey != "sk-test" {
		t.Errorf("expected fallback to openai config, got APIKey=%q", cfg.APIKey)
	}
}

// --- adapter 行为测试（HTTP mock 抓请求） ---

// mockServer 创建一个 mock HTTP server，返回 OpenAI 兼容响应，捕获请求体
func mockServer(t *testing.T) (*httptest.Server, *capturedRequest) {
	t.Helper()
	cap := &capturedRequest{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		cap.mu.Lock()
		cap.body = body
		cap.path = r.URL.Path
		cap.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		// OpenAI-compatible response
		_, _ = w.Write([]byte(`{"id":"test","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
	}))
	return srv, cap
}

type capturedRequest struct {
	mu   sync.Mutex
	body []byte
	path string
}

func (c *capturedRequest) bodyMap() map[string]any {
	c.mu.Lock()
	defer c.mu.Unlock()
	var m map[string]any
	_ = json.Unmarshal(c.body, &m)
	return m
}

func thinkingOpts(thinking bool) []model.Option {
	return []model.Option{biz.WithParams(&biz.RequestParams{Thinking: &thinking})}
}

var testMessages = []*schema.Message{{Role: schema.User, Content: "hi"}}

func TestOpenAIAdapter_NoReasoningForUnsupportedModel(t *testing.T) {
	srv, cap := mockServer(t)
	defer srv.Close()

	// gpt-4o 不支持 reasoning_effort，adapter 应该不注入
	cfg := conf.Client{BaseURL: srv.URL + "/v1", APIKey: "test"}
	cm, err := newOpenAI(context.Background(), cfg, "gpt-4o")
	if err != nil {
		t.Fatal(err)
	}

	_, err = cm.Generate(context.Background(), testMessages, thinkingOpts(true)...)
	if err != nil {
		t.Fatal(err)
	}

	body := cap.bodyMap()
	if _, ok := body["reasoning_effort"]; ok {
		t.Error("gpt-4o should not have reasoning_effort injected")
	}
}

func TestOpenAIAdapter_IsWrapped(t *testing.T) {
	srv, _ := mockServer(t)
	defer srv.Close()

	cfg := conf.Client{BaseURL: srv.URL + "/v1", APIKey: "test"}

	adapted, err := newOpenAI(context.Background(), cfg, "gpt-4o")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := newOpenAIRaw(context.Background(), cfg, "gpt-4o")
	if err != nil {
		t.Fatal(err)
	}

	// adapted 应该是 openAIAdapter，raw 应该是 SDK 原始类型
	if reflect.TypeOf(adapted) == reflect.TypeOf(raw) {
		t.Error("adapted and raw should be different types")
	}
	if _, ok := adapted.(*openAIAdapter); !ok {
		t.Errorf("adapted should be *openAIAdapter, got %T", adapted)
	}
}

func TestOpenAIRaw_NoReasoningEffort(t *testing.T) {
	srv, cap := mockServer(t)
	defer srv.Close()

	cfg := conf.Client{BaseURL: srv.URL + "/v1", APIKey: "test"}
	cm, err := newOpenAIRaw(context.Background(), cfg, "gpt-4o")
	if err != nil {
		t.Fatal(err)
	}

	_, err = cm.Generate(context.Background(), testMessages, thinkingOpts(true)...)
	if err != nil {
		t.Fatal(err)
	}

	body := cap.bodyMap()
	if _, ok := body["reasoning_effort"]; ok {
		t.Error("raw client should not inject reasoning_effort")
	}
}

func TestArkAdapter_ThinkingInjectsThinkingType(t *testing.T) {
	srv, cap := mockServer(t)
	defer srv.Close()

	cfg := conf.Client{BaseURL: srv.URL + "/api/v3", APIKey: "test"}
	cm, err := newArk(context.Background(), cfg, "ep-test-model")
	if err != nil {
		t.Fatal(err)
	}

	_, err = cm.Generate(context.Background(), testMessages, thinkingOpts(true)...)
	if err != nil {
		t.Fatal(err)
	}

	body := cap.bodyMap()
	thinking, ok := body["thinking"]
	if !ok {
		t.Fatal("expected thinking in request body, not found")
	}
	thinkingMap, ok := thinking.(map[string]any)
	if !ok {
		t.Fatalf("thinking is not a map: %T", thinking)
	}
	if thinkingMap["type"] != "enabled" {
		t.Errorf("thinking.type = %v, want enabled", thinkingMap["type"])
	}
}

func TestQwenAdapter_ThinkingInjectsEnableThinking(t *testing.T) {
	srv, cap := mockServer(t)
	defer srv.Close()

	cfg := conf.Client{BaseURL: srv.URL + "/compatible-mode/v1", APIKey: "test"}
	cm, err := newQwen(context.Background(), cfg, "qwen-plus")
	if err != nil {
		t.Fatal(err)
	}

	_, err = cm.Generate(context.Background(), testMessages, thinkingOpts(true)...)
	if err != nil {
		t.Fatal(err)
	}

	body := cap.bodyMap()
	if et, ok := body["enable_thinking"]; !ok {
		t.Error("expected enable_thinking in request body, not found")
	} else if et != true {
		t.Errorf("enable_thinking = %v, want true", et)
	}
}

func TestOpenRouterAdapter_ThinkingInjectsReasoning(t *testing.T) {
	srv, cap := mockServer(t)
	defer srv.Close()

	cfg := conf.Client{BaseURL: srv.URL + "/api/v1", APIKey: "test"}
	cm, err := newOpenRouter(context.Background(), cfg, "openrouter/openai/gpt-4o")
	if err != nil {
		t.Fatal(err)
	}

	_, err = cm.Generate(context.Background(), testMessages, thinkingOpts(true)...)
	if err != nil {
		t.Fatal(err)
	}

	body := cap.bodyMap()
	reasoning, ok := body["reasoning"]
	if !ok {
		t.Fatal("expected reasoning in request body, not found")
	}
	reasoningMap, ok := reasoning.(map[string]any)
	if !ok {
		t.Fatalf("reasoning is not a map: %T", reasoning)
	}
	if reasoningMap["effort"] != "high" {
		t.Errorf("reasoning.effort = %v, want high", reasoningMap["effort"])
	}
}
