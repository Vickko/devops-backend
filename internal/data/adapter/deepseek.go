package adapter

import (
	"context"

	"devops-backend/internal/data/message"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

// DeepSeekAdapter DeepSeek 模型适配器
// 职责：
// - 多模态过滤（仅支持文本）
// - 替换图片/音频/视频为占位符
// - DeepSeek 的 reasoning 是模型内置行为，无需配置选项
type DeepSeekAdapter struct {
	raw       model.BaseChatModel
	modelName string
}

// NewDeepSeekAdapter 创建 DeepSeekAdapter
func NewDeepSeekAdapter(raw model.BaseChatModel, modelName string) *DeepSeekAdapter {
	return &DeepSeekAdapter{
		raw:       raw,
		modelName: modelName,
	}
}

// Generate 调用原始模型的 Generate，预处理消息过滤多模态内容
func (a *DeepSeekAdapter) Generate(ctx context.Context, messages []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	// 过滤多模态内容
	filteredMessages := message.FilterMultimodalContent(messages, "deepseek")
	return a.raw.Generate(ctx, filteredMessages, opts...)
}

// Stream 流式调用，预处理消息过滤多模态内容
func (a *DeepSeekAdapter) Stream(ctx context.Context, messages []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	// 过滤多模态内容
	filteredMessages := message.FilterMultimodalContent(messages, "deepseek")
	return a.raw.Stream(ctx, filteredMessages, opts...)
}
