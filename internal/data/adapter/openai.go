package adapter

import (
	"context"
	"strings"

	"devops-backend/internal/biz"

	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

// OpenAIAdapter OpenAI 模型适配器
// 职责：
// - ReasoningEffort 配置
// - 仅 reasoning 模型（o1/o3/gpt-5 系列）支持
type OpenAIAdapter struct {
	raw       model.BaseChatModel
	modelName string
}

// NewOpenAIAdapter 创建 OpenAIAdapter
func NewOpenAIAdapter(raw model.BaseChatModel, modelName string) *OpenAIAdapter {
	return &OpenAIAdapter{
		raw:       raw,
		modelName: modelName,
	}
}

// Generate 调用原始模型的 Generate，注入 ReasoningEffort 配置
func (a *OpenAIAdapter) Generate(ctx context.Context, messages []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	opts = a.injectReasoningEffort(opts)
	return a.raw.Generate(ctx, messages, opts...)
}

// Stream 流式调用，注入 ReasoningEffort 配置
func (a *OpenAIAdapter) Stream(ctx context.Context, messages []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	opts = a.injectReasoningEffort(opts)
	return a.raw.Stream(ctx, messages, opts...)
}

// injectReasoningEffort 根据 opts 中的 RequestParams 注入 ReasoningEffort 配置
// OpenAI reasoning_effort 仅 reasoning 模型支持：o1/o3 系列、gpt-5 系列
// - nil: 不传选项，保持原生接口默认行为
// - true: 使用 HIGH reasoning effort
// - false: gpt-5.1+ 用 none，gpt-5.1 之前用 low
func (a *OpenAIAdapter) injectReasoningEffort(opts []model.Option) []model.Option {
	params := biz.GetParams(opts...)

	// 仅当 Thinking 不为 nil 且模型支持时才注入
	if params.Thinking == nil || !supportsReasoningEffort(a.modelName) {
		return opts
	}

	if *params.Thinking {
		opts = append(opts, openai.WithReasoningEffort(openai.ReasoningEffortLevelHigh))
	} else {
		// 判断是否是 gpt-5.1+ 模型
		if isGPT51OrLater(a.modelName) {
			opts = append(opts, openai.WithReasoningEffort("none"))
		} else {
			opts = append(opts, openai.WithReasoningEffort(openai.ReasoningEffortLevelLow))
		}
	}

	return opts
}

// supportsReasoningEffort 判断模型是否支持 reasoning_effort 参数
// 仅 reasoning 模型支持：o1/o3 系列、gpt-5 系列
func supportsReasoningEffort(modelName string) bool {
	m := strings.ToLower(modelName)
	// o1, o3 系列
	if strings.HasPrefix(m, "o1") || strings.HasPrefix(m, "o3") {
		return true
	}
	// gpt-5 系列
	if strings.Contains(m, "gpt-5") {
		return true
	}
	// gpt-6 及以后
	if strings.Contains(m, "gpt-6") || strings.Contains(m, "gpt-7") {
		return true
	}
	return false
}

// isGPT51OrLater 判断是否是 gpt-5.1 或更新的模型
// gpt-5.1+ 支持 reasoning_effort=none
func isGPT51OrLater(modelName string) bool {
	m := strings.ToLower(modelName)
	// gpt-5.1, gpt-5.2, gpt-5.1-xxx, gpt-5.2-xxx 等
	if strings.Contains(m, "gpt-5.1") || strings.Contains(m, "gpt-5.2") {
		return true
	}
	// gpt-6 及以后
	if strings.Contains(m, "gpt-6") || strings.Contains(m, "gpt-7") {
		return true
	}
	return false
}
