package adapter

import (
	"context"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

// GrokAdapter Grok 模型适配器
// 职责：
// - 当前接入的 OpenAI 兼容代理不支持 reasoning_effort 参数（传了会直接 400）
// - 所以这里不做 thinking 参数注入；是否“推理”由模型名本身控制（fast-reasoning vs non-reasoning）
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
	return a.raw.Generate(ctx, messages, opts...)
}

// Stream 流式调用，注入 ReasoningEffort 配置
func (a *GrokAdapter) Stream(ctx context.Context, messages []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	return a.raw.Stream(ctx, messages, opts...)
}
