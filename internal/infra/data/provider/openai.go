package provider

import (
	"context"
	"fmt"
	"strings"

	"devops-backend/internal/biz"
	"devops-backend/internal/conf"

	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	openairesponse "github.com/Vickko/eino-openai-response"
)

// newOpenAIRaw 创建原始 OpenAI client（忠实反映厂商默认行为，始终使用 Chat Completions API）
func newOpenAIRaw(ctx context.Context, cfg conf.Client, modelName string, opts ...model.Option) (model.ToolCallingChatModel, error) {
	return openai.NewChatModel(ctx, &openai.ChatModelConfig{
		BaseURL: cfg.BaseURL, APIKey: cfg.APIKey, Model: modelName,
	})
}

// newOpenAI 创建 OpenAI 模型，自动选择 Responses API 或 Chat Completions API
func newOpenAI(ctx context.Context, cfg conf.Client, modelName string, opts ...model.Option) (model.ToolCallingChatModel, error) {
	if shouldUseResponsesAPI(modelName) {
		raw, err := openairesponse.NewChatModel(ctx, &openairesponse.Config{
			BaseURL: cfg.BaseURL, APIKey: cfg.APIKey, Model: modelName,
		})
		if err != nil {
			return nil, err
		}
		return &openAIResponseAdapter{raw: raw, modelName: modelName}, nil
	}
	raw, err := openai.NewChatModel(ctx, &openai.ChatModelConfig{
		BaseURL: cfg.BaseURL, APIKey: cfg.APIKey, Model: modelName,
	})
	if err != nil {
		return nil, err
	}
	return &openAIAdapter{raw: raw, modelName: modelName}, nil
}

func shouldUseResponsesAPI(modelName string) bool {
	m := strings.ToLower(modelName)
	if strings.HasPrefix(m, "o1") || strings.HasPrefix(m, "o3") || strings.HasPrefix(m, "o4") {
		return true
	}
	return strings.HasPrefix(m, "gpt-5") || strings.HasPrefix(m, "gpt-6") || strings.HasPrefix(m, "gpt-7")
}

// --- Chat Completions adapter ---

type openAIAdapter struct {
	raw       model.ToolCallingChatModel
	modelName string
}

func (a *openAIAdapter) GetType() string {
	if c, ok := a.raw.(interface{ GetType() string }); ok {
		return c.GetType()
	}
	return "OpenAI"
}

func (a *openAIAdapter) IsCallbacksEnabled() bool {
	if c, ok := a.raw.(interface{ IsCallbacksEnabled() bool }); ok {
		return c.IsCallbacksEnabled()
	}
	return true
}

func (a *openAIAdapter) Generate(ctx context.Context, messages []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	return a.raw.Generate(ctx, messages, a.injectOpts(opts)...)
}

func (a *openAIAdapter) Stream(ctx context.Context, messages []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	return a.raw.Stream(ctx, messages, a.injectOpts(opts)...)
}

func (a *openAIAdapter) WithTools(tools []*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	m, err := a.raw.WithTools(tools)
	if err != nil {
		return nil, err
	}
	return &openAIAdapter{raw: m, modelName: a.modelName}, nil
}

func (a *openAIAdapter) injectOpts(opts []model.Option) []model.Option {
	params := biz.GetParams(opts...)
	if params.Thinking == nil || !supportsReasoningEffort(a.modelName) {
		return opts
	}
	if *params.Thinking {
		return append(opts, openai.WithReasoningEffort(openai.ReasoningEffortLevelHigh))
	}
	if isGPT51OrLater(a.modelName) {
		return append(opts, openai.WithReasoningEffort("none"))
	}
	return append(opts, openai.WithReasoningEffort(openai.ReasoningEffortLevelLow))
}

// --- Responses API adapter ---

type openAIResponseAdapter struct {
	raw       model.BaseChatModel
	modelName string
}

func (a *openAIResponseAdapter) GetType() string {
	if c, ok := a.raw.(interface{ GetType() string }); ok {
		return c.GetType()
	}
	return "OpenAI"
}

func (a *openAIResponseAdapter) IsCallbacksEnabled() bool {
	if c, ok := a.raw.(interface{ IsCallbacksEnabled() bool }); ok {
		return c.IsCallbacksEnabled()
	}
	return true
}

func (a *openAIResponseAdapter) Generate(ctx context.Context, messages []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	return a.raw.Generate(ctx, messages, a.injectOpts(opts)...)
}

func (a *openAIResponseAdapter) Stream(ctx context.Context, messages []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	return a.raw.Stream(ctx, messages, a.injectOpts(opts)...)
}

func (a *openAIResponseAdapter) WithTools(tools []*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	if tc, ok := a.raw.(model.ToolCallingChatModel); ok {
		m, err := tc.WithTools(tools)
		if err != nil {
			return nil, err
		}
		return &openAIResponseAdapter{raw: m, modelName: a.modelName}, nil
	}
	if len(tools) > 0 {
		return nil, fmt.Errorf("openAIResponseAdapter: underlying model does not support tool calling")
	}
	return a, nil
}

func (a *openAIResponseAdapter) injectOpts(opts []model.Option) []model.Option {
	params := biz.GetParams(opts...)
	if params.Thinking == nil || !supportsResponsesAPIReasoning(a.modelName) {
		return opts
	}
	if *params.Thinking {
		return append(opts, openairesponse.WithReasoningEffort(openairesponse.ReasoningEffortHigh), openairesponse.WithReasoningSummary(openairesponse.ReasoningSummaryDetailed))
	}
	return append(opts, openairesponse.WithReasoningEffort(openairesponse.ReasoningEffortLow))
}

// --- helpers ---

func supportsReasoningEffort(name string) bool {
	m := strings.ToLower(name)
	return strings.HasPrefix(m, "o1") || strings.HasPrefix(m, "o3") || strings.Contains(m, "gpt-5") || strings.Contains(m, "gpt-6") || strings.Contains(m, "gpt-7")
}

func isGPT51OrLater(name string) bool {
	m := strings.ToLower(name)
	return strings.Contains(m, "gpt-5.1") || strings.Contains(m, "gpt-5.2") || strings.Contains(m, "gpt-6") || strings.Contains(m, "gpt-7")
}

func supportsResponsesAPIReasoning(name string) bool {
	m := strings.ToLower(name)
	return strings.HasPrefix(m, "o1") || strings.HasPrefix(m, "o3") || strings.HasPrefix(m, "o4") || strings.Contains(m, "gpt-5") || strings.Contains(m, "gpt-6") || strings.Contains(m, "gpt-7")
}
