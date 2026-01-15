package adapter

import (
	"context"

	"devops-backend/internal/biz"

	"github.com/cloudwego/eino-ext/components/model/claude"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

// ClaudeAdapter Claude 模型适配器
// 职责：
// - ExtendedThinking 配置（budget 32k）
// - 仅 Thinking=true 时启用
type ClaudeAdapter struct {
	raw       model.BaseChatModel
	modelName string
}

// NewClaudeAdapter 创建 ClaudeAdapter
func NewClaudeAdapter(raw model.BaseChatModel, modelName string) *ClaudeAdapter {
	return &ClaudeAdapter{
		raw:       raw,
		modelName: modelName,
	}
}

// Generate 调用原始模型的 Generate，注入 Thinking 配置
func (a *ClaudeAdapter) Generate(ctx context.Context, messages []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	opts = a.injectThinkingConfig(opts)
	return a.raw.Generate(ctx, messages, opts...)
}

// Stream 流式调用，注入 Thinking 配置
func (a *ClaudeAdapter) Stream(ctx context.Context, messages []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	opts = a.injectThinkingConfig(opts)
	return a.raw.Stream(ctx, messages, opts...)
}

// injectThinkingConfig 根据 opts 中的 RequestParams 注入 Claude Thinking 配置
// Claude Extended thinking 需要显式启用，默认是关闭的
// - nil: 不传选项，保持原生默认行为（不启用 thinking）
// - true: 启用 extended thinking，预算 32k tokens
// - false: 不传选项（默认就是关闭）
func (a *ClaudeAdapter) injectThinkingConfig(opts []model.Option) []model.Option {
	params := biz.GetParams(opts...)

	// 仅当显式设置为 true 时才启用 extended thinking
	if params.Thinking != nil && *params.Thinking {
		opts = append(opts, claude.WithThinking(&claude.Thinking{
			Enable:       true,
			BudgetTokens: 32000, // 32k tokens 预算
		}))
	}

	return opts
}
