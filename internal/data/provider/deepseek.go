package provider

import (
	"context"

	"devops-backend/internal/conf"

	"github.com/cloudwego/eino-ext/components/model/deepseek"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

// newDeepSeekRaw 创建原始 DeepSeek client
func newDeepSeekRaw(ctx context.Context, cfg conf.Client, modelName string, opts ...model.Option) (model.ToolCallingChatModel, error) {
	return deepseek.NewChatModel(ctx, &deepseek.ChatModelConfig{
		BaseURL: cfg.BaseURL, APIKey: cfg.APIKey, Model: modelName,
	})
}

// newDeepSeek 创建 DeepSeek 模型 + 多模态过滤 adapter
func newDeepSeek(ctx context.Context, cfg conf.Client, modelName string, opts ...model.Option) (model.ToolCallingChatModel, error) {
	raw, err := deepseek.NewChatModel(ctx, &deepseek.ChatModelConfig{
		BaseURL: cfg.BaseURL, APIKey: cfg.APIKey, Model: modelName,
	})
	if err != nil {
		return nil, err
	}
	return &deepSeekAdapter{raw: raw}, nil
}

type deepSeekAdapter struct{ raw model.ToolCallingChatModel }

// GetType delegates to the underlying model so the compose framework
// recognises this adapter as the same component type.
func (a *deepSeekAdapter) GetType() string {
	if c, ok := a.raw.(interface{ GetType() string }); ok {
		return c.GetType()
	}
	return "DeepSeek"
}

// IsCallbacksEnabled delegates to the underlying model.
// Without this the compose framework adds its own callback layer on top of
// the raw client's internal callbacks, causing every streaming event to be
// emitted twice.
func (a *deepSeekAdapter) IsCallbacksEnabled() bool {
	if c, ok := a.raw.(interface{ IsCallbacksEnabled() bool }); ok {
		return c.IsCallbacksEnabled()
	}
	return true
}

func (a *deepSeekAdapter) Generate(ctx context.Context, messages []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	return a.raw.Generate(ctx, FilterMultimodalContent(messages, "deepseek"), opts...)
}

func (a *deepSeekAdapter) Stream(ctx context.Context, messages []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	return a.raw.Stream(ctx, FilterMultimodalContent(messages, "deepseek"), opts...)
}

func (a *deepSeekAdapter) WithTools(tools []*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	m, err := a.raw.WithTools(tools)
	if err != nil {
		return nil, err
	}
	return &deepSeekAdapter{raw: m}, nil
}
