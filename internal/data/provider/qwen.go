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
func newQwenRaw(ctx context.Context, cfg conf.Client, modelName string, opts ...model.Option) (model.BaseChatModel, error) {
	return qwen.NewChatModel(ctx, &qwen.ChatModelConfig{
		BaseURL: cfg.BaseURL, APIKey: cfg.APIKey, Model: modelName,
	})
}

// newQwen 创建 Qwen 模型 + thinking adapter
func newQwen(ctx context.Context, cfg conf.Client, modelName string, opts ...model.Option) (model.BaseChatModel, error) {
	raw, err := newQwenRaw(ctx, cfg, modelName, opts...)
	if err != nil {
		return nil, err
	}
	return &qwenAdapter{raw: raw}, nil
}

type qwenAdapter struct{ raw model.BaseChatModel }

func (a *qwenAdapter) Generate(ctx context.Context, messages []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	return a.raw.Generate(ctx, messages, a.injectOpts(opts)...)
}

func (a *qwenAdapter) Stream(ctx context.Context, messages []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	return a.raw.Stream(ctx, messages, a.injectOpts(opts)...)
}

func (a *qwenAdapter) injectOpts(opts []model.Option) []model.Option {
	params := biz.GetParams(opts...)
	if params.Thinking == nil {
		return opts
	}
	return append(opts, qwen.WithEnableThinking(*params.Thinking))
}
