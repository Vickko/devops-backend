package provider

import (
	"reflect"
	"runtime"
	"testing"

	"devops-backend/internal/conf"
)

// funcName 获取函数名用于断言
func funcName(f createFunc) string {
	return runtime.FuncForPC(reflect.ValueOf(f).Pointer()).Name()
}

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
