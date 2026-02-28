package provider

import (
	"context"
	"strings"

	"devops-backend/internal/conf"

	"github.com/cloudwego/eino/components/model"
)

// createFunc 创建 ChatModel 的工厂函数
type createFunc func(ctx context.Context, cfg conf.Client, modelName string, opts ...model.Option) (model.ToolCallingChatModel, error)

type providerEntry struct {
	clientName string
	keywords   []string
	create     createFunc // 带 adapter
	createRaw  createFunc // 原始 client（nil 表示无 adapter，等同于 create）
}

// MixedProvider 混合提供者，按模型名自动路由
type MixedProvider struct {
	entries   []providerEntry
	clients   map[string]conf.Client
	overrides map[string]string // model name → client name
	fallback  createFunc
}

// NewMixedProvider 创建混合提供者
func NewMixedProvider(cfg conf.Eino) *MixedProvider {
	return &MixedProvider{
		clients:   cfg.Clients,
		overrides: cfg.ModelOverrides,
		fallback:  newOpenAICompatible,
		entries: []providerEntry{
			// 前缀匹配优先（避免被通用关键词抢走）
			{"openrouter", []string{"openrouter/"}, newOpenRouter, newOpenRouterRaw},
			{"arkbot", []string{"bot-"}, newArkBot, nil},
			{"ark", []string{"ep-", "doubao"}, newArk, newArkRaw},
			// 通用关键词匹配
			{"openai", []string{"gpt", "o1", "o3", "o4", "chatgpt", "llama"}, newOpenAI, newOpenAIRaw},
			{"claude", []string{"claude"}, newClaude, newClaudeRaw},
			{"deepseek", []string{"deepseek"}, newDeepSeek, newDeepSeekRaw},
			{"gemini", []string{"gemini"}, newGemini, newGeminiRaw},
			{"grok", []string{"grok"}, newOpenAICompatible, nil},
			{"qianfan", []string{"ernie", "qianfan"}, newQianfan, nil},
			{"qwen", []string{"qwen"}, newQwen, newQwenRaw},
			{"glm", []string{"glm"}, newOpenAICompatible, nil},
			{"kimi", []string{"kimi"}, newOpenAICompatible, nil},
			{"minimax", []string{"minimax"}, newOpenAICompatible, nil},
		},
	}
}

// CreateChatModel 根据 modelName 自动路由，带 adapter
func (m *MixedProvider) CreateChatModel(ctx context.Context, modelName string, opts ...model.Option) (model.ToolCallingChatModel, error) {
	create, cfg := m.resolve(modelName, false)
	return create(ctx, cfg, modelName, opts...)
}

// CreateRawChatModel 根据 modelName 自动路由，返回原始 client（不包装 adapter）
func (m *MixedProvider) CreateRawChatModel(ctx context.Context, modelName string, opts ...model.Option) (model.ToolCallingChatModel, error) {
	create, cfg := m.resolve(modelName, true)
	return create(ctx, cfg, modelName, opts...)
}

func (m *MixedProvider) resolve(modelName string, raw bool) (createFunc, conf.Client) {
	// override 优先：精确匹配 model name → 强制导流到指定 client
	if target, ok := m.overrides[modelName]; ok {
		for _, e := range m.entries {
			if e.clientName == target {
				fn := e.create
				if raw && e.createRaw != nil {
					fn = e.createRaw
				}
				return fn, m.clientConfig(e.clientName)
			}
		}
		// override 指向的 client 不在注册表中，走 fallback + 目标 config
		return m.fallback, m.clientConfig(target)
	}
	// keyword 匹配
	modelLower := strings.ToLower(modelName)
	for _, e := range m.entries {
		for _, kw := range e.keywords {
			if strings.Contains(modelLower, strings.ToLower(kw)) {
				fn := e.create
				if raw && e.createRaw != nil {
					fn = e.createRaw
				}
				return fn, m.clientConfig(e.clientName)
			}
		}
	}
	return m.fallback, m.clientConfig("openai")
}

func (m *MixedProvider) clientConfig(name string) conf.Client {
	cfg, ok := m.clients[name]
	if !ok || (name != "openai" && cfg.BaseURL == "" && cfg.APIKey == "") {
		if oc, exists := m.clients["openai"]; exists && (oc.BaseURL != "" || oc.APIKey != "") {
			return oc
		}
	}
	return cfg
}
