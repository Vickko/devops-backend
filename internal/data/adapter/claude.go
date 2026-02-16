package adapter

import (
	"context"
	"errors"
	"io"

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
	resp, err := a.raw.Generate(ctx, messages, opts...)
	if err == nil {
		return resp, nil
	}

	// 部分代理/网关对 Claude 的非流式接口支持不好，Generate 可能直接报错。
	// 这里自动 fallback：用 Stream 收集成完整 Message 返回。
	sr, streamErr := a.raw.Stream(ctx, messages, opts...)
	if streamErr != nil {
		return nil, streamErr
	}
	defer sr.Close()

	collected, collectErr := collectStreamToMessage(sr)
	if collectErr != nil {
		return nil, collectErr
	}
	return collected, nil
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

		if chunk.Content != "" {
			full.Content += chunk.Content
		}
		if chunk.ReasoningContent != "" {
			full.ReasoningContent += chunk.ReasoningContent
		}
		if len(chunk.AssistantGenMultiContent) > 0 {
			full.AssistantGenMultiContent = append(full.AssistantGenMultiContent, chunk.AssistantGenMultiContent...)
		}
		if len(chunk.ToolCalls) > 0 {
			// keep latest tool calls snapshot (if any)
			full.ToolCalls = chunk.ToolCalls
		}
	}

	return &full, nil
}
