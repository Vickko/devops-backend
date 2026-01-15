package data

import (
	"context"
	"fmt"
	"strings"

	"devops-backend/internal/biz"
	"devops-backend/internal/conf"
	"devops-backend/internal/data/adapter"
	openairesponse "devops-backend/internal/data/model/openai-response"

	"github.com/cloudwego/eino-ext/components/model/ark"
	"github.com/cloudwego/eino-ext/components/model/arkbot"
	"github.com/cloudwego/eino-ext/components/model/claude"
	"github.com/cloudwego/eino-ext/components/model/deepseek"
	"github.com/cloudwego/eino-ext/components/model/gemini"
	"github.com/cloudwego/eino-ext/components/model/ollama"
	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino-ext/components/model/openrouter"
	"github.com/cloudwego/eino-ext/components/model/qianfan"
	"github.com/cloudwego/eino-ext/components/model/qwen"
	"github.com/cloudwego/eino/components/model"
	"google.golang.org/genai"
)

// 客户端关键词映射（硬编码）
var clientKeywords = map[string][]string{
	"openai":     {"gpt", "o1", "o3", "o4", "chatgpt"},
	"claude":     {"claude"},
	"ark":        {"doubao", "ep-"},
	"arkbot":     {"bot-"},
	"deepseek":   {"deepseek"},
	"gemini":     {"gemini"},
	"grok":       {"grok"},
	"glm":        {"glm"},
	"kimi":       {"kimi"},
	"minimax":    {"minimax"},
	"ollama":     {"llama", "gemma", "phi", "mistral", "codellama", "vicuna"},
	"openrouter": {"openrouter/"},
	"qianfan":    {"ernie", "qianfan"},
	"qwen":       {"qwen"},
}

// shouldUseResponsesAPI 判断模型是否应使用 Responses API
// OpenAI 推荐对新项目使用 Responses API，且 o-series 和 gpt-5+ 的 reasoning 功能仅在 Responses API 中可用
func shouldUseResponsesAPI(modelName string) bool {
	modelLower := strings.ToLower(modelName)

	// o-series: o1, o1-mini, o1-pro, o3, o3-mini, o4-mini 等
	if strings.HasPrefix(modelLower, "o1") ||
		strings.HasPrefix(modelLower, "o3") ||
		strings.HasPrefix(modelLower, "o4") {
		return true
	}

	// gpt-5 及以后版本
	if strings.HasPrefix(modelLower, "gpt-5") ||
		strings.HasPrefix(modelLower, "gpt-6") ||
		strings.HasPrefix(modelLower, "gpt-7") {
		return true
	}

	return false
}

// ClientFactory 客户端工厂
type ClientFactory struct {
	clients map[string]conf.Client
}

// NewClientFactory 创建客户端工厂
func NewClientFactory(cfg conf.Eino) biz.ChatModelFactory {
	return &ClientFactory{
		clients: cfg.Clients,
	}
}

// CreateChatModel 根据客户端名称和模型创建 ChatModel
// useAdapter: true 时使用 adapter 包装（自动处理 thinking 等特性），false 时返回原始模型
func (f *ClientFactory) CreateChatModel(ctx context.Context, clientName, modelName string, useAdapter bool) (model.BaseChatModel, error) {
	client, ok := f.clients[clientName]
	if !ok {
		return nil, fmt.Errorf("unknown client: %s", clientName)
	}

	// 非 openai 客户端配置为空时，fallback 到 openai 配置
	if clientName != "openai" && client.BaseURL == "" && client.APIKey == "" {
		if openaiClient, exists := f.clients["openai"]; exists && (openaiClient.BaseURL != "" || openaiClient.APIKey != "") {
			client = openaiClient
		}
	}

	var raw model.BaseChatModel
	var err error

	switch clientName {
	case "openai":
		// o-series 和 gpt-5+ 自动切换到 Responses API 以支持 reasoning
		if shouldUseResponsesAPI(modelName) {
			raw, err = openairesponse.NewChatModel(ctx, &openairesponse.Config{
				BaseURL: client.BaseURL,
				APIKey:  client.APIKey,
				Model:   modelName,
			})
			if err != nil {
				return nil, err
			}
			if useAdapter {
				return adapter.NewOpenAIResponseAdapter(raw, modelName), nil
			}
			return raw, nil
		}
		// 其他模型使用 Chat Completions API
		raw, err = openai.NewChatModel(ctx, &openai.ChatModelConfig{
			BaseURL: client.BaseURL,
			APIKey:  client.APIKey,
			Model:   modelName,
		})
		if err != nil {
			return nil, err
		}
		if useAdapter {
			return adapter.NewOpenAIAdapter(raw, modelName), nil
		}
		return raw, nil

	case "openai-response":
		// OpenAI Responses API - 支持 reasoning.summary 获取思考过程
		raw, err = openairesponse.NewChatModel(ctx, &openairesponse.Config{
			BaseURL: client.BaseURL,
			APIKey:  client.APIKey,
			Model:   modelName,
		})
		if err != nil {
			return nil, err
		}
		if useAdapter {
			return adapter.NewOpenAIResponseAdapter(raw, modelName), nil
		}
		return raw, nil

	case "claude":
		raw, err = claude.NewChatModel(ctx, &claude.Config{
			BaseURL:   &client.BaseURL,
			APIKey:    client.APIKey,
			Model:     modelName,
			MaxTokens: 40000, // 需大于 thinking.budget_tokens (32k)
		})
		if err != nil {
			return nil, err
		}
		if useAdapter {
			return adapter.NewClaudeAdapter(raw, modelName), nil
		}
		return raw, nil

	case "ark":
		return ark.NewChatModel(ctx, &ark.ChatModelConfig{
			BaseURL: client.BaseURL,
			APIKey:  client.APIKey,
			Model:   modelName,
		})

	case "arkbot":
		return arkbot.NewChatModel(ctx, &arkbot.Config{
			BaseURL: client.BaseURL,
			APIKey:  client.APIKey,
			Model:   modelName,
		})

	case "deepseek":
		raw, err = deepseek.NewChatModel(ctx, &deepseek.ChatModelConfig{
			BaseURL: client.BaseURL,
			APIKey:  client.APIKey,
			Model:   modelName,
		})
		if err != nil {
			return nil, err
		}
		if useAdapter {
			return adapter.NewDeepSeekAdapter(raw, modelName), nil
		}
		return raw, nil

	case "gemini":
		genaiClient, err := genai.NewClient(ctx, &genai.ClientConfig{
			APIKey: client.APIKey,
			HTTPOptions: genai.HTTPOptions{
				BaseURL: client.BaseURL,
			},
		})
		if err != nil {
			return nil, fmt.Errorf("create gemini client: %w", err)
		}
		raw, err = gemini.NewChatModel(ctx, &gemini.Config{
			Client: genaiClient,
			Model:  modelName,
			ResponseModalities: []gemini.GeminiResponseModality{
				gemini.GeminiResponseModalityText,
				gemini.GeminiResponseModalityImage,
			},
		})
		if err != nil {
			return nil, err
		}
		if useAdapter {
			return adapter.NewGeminiAdapter(raw, modelName), nil
		}
		return raw, nil

	case "ollama":
		return ollama.NewChatModel(ctx, &ollama.ChatModelConfig{
			BaseURL: client.BaseURL,
			Model:   modelName,
		})

	case "openrouter":
		return openrouter.NewChatModel(ctx, &openrouter.Config{
			BaseURL: client.BaseURL,
			APIKey:  client.APIKey,
			Model:   modelName,
		})

	case "qianfan":
		if client.APIKey != "" {
			cfg := qianfan.GetQianfanSingletonConfig()
			parts := strings.SplitN(client.APIKey, ":", 2)
			if len(parts) == 2 {
				cfg.AccessKey = parts[0]
				cfg.SecretKey = parts[1]
			}
		}
		if client.BaseURL != "" {
			qianfan.GetQianfanSingletonConfig().BaseURL = client.BaseURL
		}
		return qianfan.NewChatModel(ctx, &qianfan.ChatModelConfig{
			Model: modelName,
		})

	case "qwen":
		return qwen.NewChatModel(ctx, &qwen.ChatModelConfig{
			BaseURL: client.BaseURL,
			APIKey:  client.APIKey,
			Model:   modelName,
		})

	case "grok":
		// Grok 使用 OpenAI 兼容接口，支持 reasoning_effort
		raw, err = openai.NewChatModel(ctx, &openai.ChatModelConfig{
			BaseURL: client.BaseURL,
			APIKey:  client.APIKey,
			Model:   modelName,
		})
		if err != nil {
			return nil, err
		}
		if useAdapter {
			return adapter.NewGrokAdapter(raw, modelName), nil
		}
		return raw, nil

	case "glm":
		// GLM 使用 OpenAI 兼容接口，thinking 为模型内置行为
		return openai.NewChatModel(ctx, &openai.ChatModelConfig{
			BaseURL: client.BaseURL,
			APIKey:  client.APIKey,
			Model:   modelName,
		})

	case "kimi":
		// Kimi 使用 OpenAI 兼容接口，thinking 为模型内置行为
		return openai.NewChatModel(ctx, &openai.ChatModelConfig{
			BaseURL: client.BaseURL,
			APIKey:  client.APIKey,
			Model:   modelName,
		})

	case "minimax":
		// MiniMax 使用 OpenAI 兼容接口，thinking 为模型内置行为
		return openai.NewChatModel(ctx, &openai.ChatModelConfig{
			BaseURL: client.BaseURL,
			APIKey:  client.APIKey,
			Model:   modelName,
		})

	default:
		return openai.NewChatModel(ctx, &openai.ChatModelConfig{
			BaseURL: client.BaseURL,
			APIKey:  client.APIKey,
			Model:   modelName,
		})
	}
}

// ResolveClient 根据模型名称解析应使用的客户端
// 优先级：requestClient > 关键词匹配 > openai
func (f *ClientFactory) ResolveClient(modelName, requestClient string) string {
	// 1. 请求中显式指定的客户端优先
	if requestClient != "" {
		if _, ok := f.clients[requestClient]; ok {
			return requestClient
		}
	}

	// 2. 根据模型名称关键词匹配
	modelLower := strings.ToLower(modelName)
	for clientName, keywords := range clientKeywords {
		for _, keyword := range keywords {
			if strings.Contains(modelLower, strings.ToLower(keyword)) {
				// 确保该客户端已配置
				if _, ok := f.clients[clientName]; ok {
					return clientName
				}
			}
		}
	}

	// 3. 默认使用 openai
	return "openai"
}
