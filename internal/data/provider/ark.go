package provider

import (
	"context"

	"devops-backend/internal/biz"
	"devops-backend/internal/conf"

	"github.com/cloudwego/eino-ext/components/model/ark"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	arkModel "github.com/volcengine/volcengine-go-sdk/service/arkruntime/model"
)

// newArkRaw 创建原始 Ark client
func newArkRaw(ctx context.Context, cfg conf.Client, modelName string, opts ...model.Option) (model.BaseChatModel, error) {
	return ark.NewChatModel(ctx, &ark.ChatModelConfig{
		BaseURL: cfg.BaseURL, APIKey: cfg.APIKey, Model: modelName,
	})
}

// newArk 创建 Ark 模型 + thinking adapter
func newArk(ctx context.Context, cfg conf.Client, modelName string, opts ...model.Option) (model.BaseChatModel, error) {
	raw, err := newArkRaw(ctx, cfg, modelName, opts...)
	if err != nil {
		return nil, err
	}
	return &arkAdapter{raw: raw}, nil
}

type arkAdapter struct{ raw model.BaseChatModel }

func (a *arkAdapter) Generate(ctx context.Context, messages []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	return a.raw.Generate(ctx, messages, a.injectOpts(opts)...)
}

func (a *arkAdapter) Stream(ctx context.Context, messages []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	return a.raw.Stream(ctx, messages, a.injectOpts(opts)...)
}

func (a *arkAdapter) injectOpts(opts []model.Option) []model.Option {
	params := biz.GetParams(opts...)
	if params.Thinking == nil {
		return opts
	}
	if *params.Thinking {
		return append(opts, ark.WithThinking(&arkModel.Thinking{Type: arkModel.ThinkingTypeEnabled}))
	}
	return append(opts, ark.WithThinking(&arkModel.Thinking{Type: arkModel.ThinkingTypeDisabled}))
}
