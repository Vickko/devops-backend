package adapter

import (
	"context"
	"strings"

	"devops-backend/internal/biz"

	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

// GrokAdapter Grok 模型适配器
// 职责：
// - ReasoningEffort 配置（仅 reasoning 模型支持）
// - Grok 的 reasoning 模型名称包含 "reasoning" 关键词
type GrokAdapter struct {
	raw       model.BaseChatModel
	modelName string
}

// NewGrokAdapter 创建 GrokAdapter
func NewGrokAdapter(raw model.BaseChatModel, modelName string) *GrokAdapter {
	return &GrokAdapter{
		raw:       raw,
		modelName: modelName,
	}
}

// Generate 调用原始模型的 Generate，注入 ReasoningEffort 配置
func (a *GrokAdapter) Generate(ctx context.Context, messages []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	opts = a.injectReasoningEffort(opts)
	return a.raw.Generate(ctx, messages, opts...)
}

// Stream 流式调用，注入 ReasoningEffort 配置
func (a *GrokAdapter) Stream(ctx context.Context, messages []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	opts = a.injectReasoningEffort(opts)
	return a.raw.Stream(ctx, messages, opts...)
}

// injectReasoningEffort 根据 opts 中的 RequestParams 注入 ReasoningEffort 配置
// Grok reasoning_effort 仅 reasoning 模型支持（模型名称包含 "reasoning"）
// - nil: 不传选项，保持原生接口默认行为
// - true: 使用 HIGH reasoning effort
// - false: 使用 LOW reasoning effort
func (a *GrokAdapter) injectReasoningEffort(opts []model.Option) []model.Option {
	params := biz.GetParams(opts...)

	// 仅当 Thinking 不为 nil 且模型支持时才注入
	if params.Thinking == nil || !isGrokReasoningModel(a.modelName) {
		return opts
	}

	if *params.Thinking {
		opts = append(opts, openai.WithReasoningEffort(openai.ReasoningEffortLevelHigh))
	} else {
		opts = append(opts, openai.WithReasoningEffort(openai.ReasoningEffortLevelLow))
	}

	return opts
}

// isGrokReasoningModel 判断是否是 Grok reasoning 模型
// Grok 的 reasoning 模型名称包含 "reasoning" 关键词
func isGrokReasoningModel(modelName string) bool {
	return strings.Contains(strings.ToLower(modelName), "reasoning")
}
