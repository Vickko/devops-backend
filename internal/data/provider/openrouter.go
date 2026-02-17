package provider

import (
	"context"

	"devops-backend/internal/biz"
	"devops-backend/internal/conf"

	"github.com/cloudwego/eino-ext/components/model/openrouter"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

// newOpenRouterRaw 创建原始 OpenRouter client
func newOpenRouterRaw(ctx context.Context, cfg conf.Client, modelName string, opts ...model.Option) (model.ToolCallingChatModel, error) {
	return openrouter.NewChatModel(ctx, &openrouter.Config{
		BaseURL: cfg.BaseURL, APIKey: cfg.APIKey, Model: modelName,
	})
}

// newOpenRouter 创建 OpenRouter 模型 + reasoning adapter
func newOpenRouter(ctx context.Context, cfg conf.Client, modelName string, opts ...model.Option) (model.ToolCallingChatModel, error) {
	raw, err := newOpenRouterRaw(ctx, cfg, modelName, opts...)
	if err != nil {
		return nil, err
	}
	return &openRouterAdapter{raw: raw}, nil
}

type openRouterAdapter struct{ raw model.ToolCallingChatModel }

func (a *openRouterAdapter) Generate(ctx context.Context, messages []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	return a.raw.Generate(ctx, messages, a.injectOpts(opts)...)
}

func (a *openRouterAdapter) Stream(ctx context.Context, messages []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	return a.raw.Stream(ctx, messages, a.injectOpts(opts)...)
}

func (a *openRouterAdapter) WithTools(tools []*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	m, err := a.raw.WithTools(tools)
	if err != nil {
		return nil, err
	}
	return &openRouterAdapter{raw: m}, nil
}

func (a *openRouterAdapter) injectOpts(opts []model.Option) []model.Option {
	params := biz.GetParams(opts...)
	if params.Thinking == nil {
		return opts
	}
	if *params.Thinking {
		return append(opts, openrouter.WithReasoning(&openrouter.Reasoning{
			Effort: openrouter.EffortOfHigh, Summary: openrouter.SummaryOfDetailed,
		}))
	}
	return append(opts, openrouter.WithReasoning(&openrouter.Reasoning{
		Effort: openrouter.EffortOfLow,
	}))
}
