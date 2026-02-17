package provider

import (
	"context"
	"errors"
	"io"

	"devops-backend/internal/biz"
	"devops-backend/internal/conf"

	"github.com/cloudwego/eino-ext/components/model/claude"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

// newClaudeRaw 创建原始 Claude client（忠实反映厂商默认行为）
func newClaudeRaw(ctx context.Context, cfg conf.Client, modelName string, opts ...model.Option) (model.ToolCallingChatModel, error) {
	return claude.NewChatModel(ctx, &claude.Config{
		BaseURL: &cfg.BaseURL, APIKey: cfg.APIKey, Model: modelName,
	})
}

// newClaude 创建 Claude 模型 + thinking adapter
func newClaude(ctx context.Context, cfg conf.Client, modelName string, opts ...model.Option) (model.ToolCallingChatModel, error) {
	raw, err := claude.NewChatModel(ctx, &claude.Config{
		BaseURL: &cfg.BaseURL, APIKey: cfg.APIKey, Model: modelName, MaxTokens: 40000,
	})
	if err != nil {
		return nil, err
	}
	return &claudeAdapter{raw: raw, modelName: modelName}, nil
}

type claudeAdapter struct {
	raw       model.ToolCallingChatModel
	modelName string
}

func (a *claudeAdapter) Generate(ctx context.Context, messages []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	opts = a.injectOpts(opts)
	resp, err := a.raw.Generate(ctx, messages, opts...)
	if err == nil {
		return resp, nil
	}
	// fallback: Stream → collect
	sr, streamErr := a.raw.Stream(ctx, messages, opts...)
	if streamErr != nil {
		return nil, streamErr
	}
	defer sr.Close()
	return collectStreamToMessage(sr)
}

func (a *claudeAdapter) Stream(ctx context.Context, messages []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	return a.raw.Stream(ctx, messages, a.injectOpts(opts)...)
}

func (a *claudeAdapter) WithTools(tools []*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	m, err := a.raw.WithTools(tools)
	if err != nil {
		return nil, err
	}
	return &claudeAdapter{raw: m, modelName: a.modelName}, nil
}

func (a *claudeAdapter) injectOpts(opts []model.Option) []model.Option {
	params := biz.GetParams(opts...)
	if params.Thinking != nil && *params.Thinking {
		opts = append(opts, claude.WithThinking(&claude.Thinking{Enable: true, BudgetTokens: 32000}))
	}
	return opts
}

func collectStreamToMessage(sr *schema.StreamReader[*schema.Message]) (*schema.Message, error) {
	var full schema.Message
	full.Role = schema.Assistant
	for {
		chunk, err := sr.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		full.Content += chunk.Content
		full.ReasoningContent += chunk.ReasoningContent
		if len(chunk.AssistantGenMultiContent) > 0 {
			full.AssistantGenMultiContent = append(full.AssistantGenMultiContent, chunk.AssistantGenMultiContent...)
		}
		if len(chunk.ToolCalls) > 0 {
			full.ToolCalls = chunk.ToolCalls
		}
	}
	return &full, nil
}
