package provider

import (
	"context"

	"devops-backend/internal/biz"
	"devops-backend/internal/conf"

	"github.com/cloudwego/eino-ext/components/model/qwen"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

// newQwenRaw 创建原始 Qwen client
func newQwenRaw(ctx context.Context, cfg conf.Client, modelName string, opts ...model.Option) (model.ToolCallingChatModel, error) {
	return qwen.NewChatModel(ctx, &qwen.ChatModelConfig{
		BaseURL: cfg.BaseURL, APIKey: cfg.APIKey, Model: modelName,
	})
}

// newQwen 创建 Qwen 模型 + thinking adapter
func newQwen(ctx context.Context, cfg conf.Client, modelName string, opts ...model.Option) (model.ToolCallingChatModel, error) {
	raw, err := newQwenRaw(ctx, cfg, modelName, opts...)
	if err != nil {
		return nil, err
	}
	return &qwenAdapter{raw: raw}, nil
}

type qwenAdapter struct{ raw model.ToolCallingChatModel }

func (a *qwenAdapter) GetType() string {
	if c, ok := a.raw.(interface{ GetType() string }); ok {
		return c.GetType()
	}
	return "Qwen"
}

func (a *qwenAdapter) IsCallbacksEnabled() bool {
	if c, ok := a.raw.(interface{ IsCallbacksEnabled() bool }); ok {
		return c.IsCallbacksEnabled()
	}
	return true
}

func (a *qwenAdapter) Generate(ctx context.Context, messages []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	return a.raw.Generate(ctx, messages, a.injectOpts(opts)...)
}

func (a *qwenAdapter) Stream(ctx context.Context, messages []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	return a.raw.Stream(ctx, messages, a.injectOpts(opts)...)
}

func (a *qwenAdapter) WithTools(tools []*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	m, err := a.raw.WithTools(tools)
	if err != nil {
		return nil, err
	}
	return &qwenAdapter{raw: m}, nil
}

func (a *qwenAdapter) injectOpts(opts []model.Option) []model.Option {
	params := biz.GetParams(opts...)
	if params.Thinking == nil {
		return opts
	}
	return append(opts, qwen.WithEnableThinking(*params.Thinking))
}
