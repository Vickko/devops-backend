package adapter

import (
	"context"
	"strings"

	"devops-backend/internal/biz"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	openairesponse "github.com/Vickko/eino-openai-response"
)

// OpenAIResponseAdapter OpenAI Responses API 模型适配器
// 职责：
// - 将 Thinking 参数转换为 Responses API 的 reasoning 配置
// - reasoning.effort + reasoning.summary 配置
// - 仅 reasoning 模型（o1/o3/o4/gpt-5+ 系列）支持
type OpenAIResponseAdapter struct {
	raw       model.BaseChatModel
	modelName string
}

// NewOpenAIResponseAdapter 创建 OpenAIResponseAdapter
func NewOpenAIResponseAdapter(raw model.BaseChatModel, modelName string) *OpenAIResponseAdapter {
	return &OpenAIResponseAdapter{
		raw:       raw,
		modelName: modelName,
	}
}

// Generate 调用原始模型的 Generate，注入 Reasoning 配置
func (a *OpenAIResponseAdapter) Generate(ctx context.Context, messages []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	opts = a.injectReasoningConfig(opts)
	return a.raw.Generate(ctx, messages, opts...)
}

// Stream 流式调用，注入 Reasoning 配置
func (a *OpenAIResponseAdapter) Stream(ctx context.Context, messages []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	opts = a.injectReasoningConfig(opts)
	return a.raw.Stream(ctx, messages, opts...)
}

// injectReasoningConfig 根据 opts 中的 RequestParams 注入 Reasoning 配置
// OpenAI Responses API 的 reasoning 配置:
// - reasoning.effort: low, medium, high (控制推理 token 数量)
// - reasoning.summary: auto, concise, detailed (控制推理摘要输出)
//
// Thinking 参数转换规则:
// - nil: 不传选项，保持原生接口默认行为
// - true: reasoning.effort=high, reasoning.summary=detailed
// - false: reasoning.effort=low (不返回推理摘要)
func (a *OpenAIResponseAdapter) injectReasoningConfig(opts []model.Option) []model.Option {
	params := biz.GetParams(opts...)

	// 仅当 Thinking 不为 nil 且模型支持时才注入
	if params.Thinking == nil || !supportsResponsesAPIReasoning(a.modelName) {
		return opts
	}

	if *params.Thinking {
		// 启用思考模式: 高推理努力 + 详细摘要
		opts = append(opts,
			openairesponse.WithReasoningEffort(openairesponse.ReasoningEffortHigh),
			openairesponse.WithReasoningSummary(openairesponse.ReasoningSummaryDetailed),
		)
	} else {
		// 禁用思考模式: 低推理努力 (不需要设置 summary，因为 low 模式不产生有意义的推理)
		opts = append(opts,
			openairesponse.WithReasoningEffort(openairesponse.ReasoningEffortLow),
		)
	}

	return opts
}

// supportsResponsesAPIReasoning 判断模型是否支持 Responses API 的 reasoning 参数
// 仅 reasoning 模型支持：o1/o3/o4 系列、gpt-5+ 系列
func supportsResponsesAPIReasoning(modelName string) bool {
	m := strings.ToLower(modelName)
	// o1, o3, o4 系列
	if strings.HasPrefix(m, "o1") || strings.HasPrefix(m, "o3") || strings.HasPrefix(m, "o4") {
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
