package adapter

import (
	"context"
	"strings"

	"devops-backend/internal/biz"
	"devops-backend/internal/data/message"

	"github.com/cloudwego/eino-ext/components/model/gemini"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"google.golang.org/genai"
)

// GeminiAdapter Gemini 模型适配器
// 职责：
// - ThinkingConfig 配置（ThinkingLevelHigh/Low）
// - RequiresNonStreamingMode fallback（调用 Generate 模拟流式）
type GeminiAdapter struct {
	raw       model.BaseChatModel
	modelName string
}

// NewGeminiAdapter 创建 GeminiAdapter
func NewGeminiAdapter(raw model.BaseChatModel, modelName string) *GeminiAdapter {
	return &GeminiAdapter{
		raw:       raw,
		modelName: modelName,
	}
}

// Generate 调用原始模型的 Generate，注入 ThinkingConfig
func (a *GeminiAdapter) Generate(ctx context.Context, messages []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	params := biz.GetParams(opts...)
	opts = a.injectThinkingConfig(opts)
	resp, err := a.raw.Generate(ctx, messages, opts...)
	if err != nil {
		return nil, err
	}

	// Gemini 在部分模型上即使 IncludeThoughts=false 也可能返回 reasoning 字段。
	// 用户关闭 Thinking 时，我们直接不展示这部分内容。
	if params.Thinking != nil && !*params.Thinking {
		resp.ReasoningContent = ""
	}
	return resp, nil
}

// Stream 流式调用，支持 fallback 到非流式模式
func (a *GeminiAdapter) Stream(ctx context.Context, messages []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	params := biz.GetParams(opts...)
	// 检查是否需要 fallback 到非流式模式
	registry := message.GetModelCapabilityRegistry()
	if registry.RequiresNonStreamingMode(a.modelName) {
		sr, err := a.streamWithFallback(ctx, messages, opts...)
		if err != nil {
			return nil, err
		}
		return wrapGeminiHideThinkingIfNeeded(sr, params), nil
	}

	opts = a.injectThinkingConfig(opts)
	sr, err := a.raw.Stream(ctx, messages, opts...)
	if err != nil {
		return nil, err
	}
	return wrapGeminiHideThinkingIfNeeded(sr, params), nil
}

// streamWithFallback 使用 Generate() 获取完整响应，然后模拟流式返回
func (a *GeminiAdapter) streamWithFallback(ctx context.Context, messages []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	opts = a.injectThinkingConfig(opts)

	// 调用 Generate 获取完整响应
	resp, err := a.raw.Generate(ctx, messages, opts...)
	if err != nil {
		return nil, err
	}

	// 创建模拟流式 reader
	return createSimulatedStreamReader(resp), nil
}

func wrapGeminiHideThinkingIfNeeded(sr *schema.StreamReader[*schema.Message], params *biz.RequestParams) *schema.StreamReader[*schema.Message] {
	if params == nil || params.Thinking == nil || *params.Thinking {
		return sr
	}

	return schema.StreamReaderWithConvert(sr, func(m *schema.Message) (*schema.Message, error) {
		if m == nil || m.ReasoningContent == "" {
			return m, nil
		}
		outCopy := *m
		outCopy.ReasoningContent = ""
		if outCopy.Content == "" && len(outCopy.AssistantGenMultiContent) == 0 {
			return nil, schema.ErrNoValue
		}
		return &outCopy, nil
	})
}

// injectThinkingConfig 根据 opts 中的 RequestParams 注入 Gemini ThinkingConfig
func (a *GeminiAdapter) injectThinkingConfig(opts []model.Option) []model.Option {
	params := biz.GetParams(opts...)

	// 一些 Gemini 图像生成模型不支持 thinking level 配置（会直接报错）。
	// 这类模型上只设置 IncludeThoughts / Budget，不设置 ThinkingLevel，保证：
	// - Thinking=false 时不展示 thinking 内容
	// - Thinking=true 时尽量展示（如果模型支持）
	registry := message.GetModelCapabilityRegistry()
	if registry.RequiresNonStreamingMode(a.modelName) {
		includeThoughts := params.Thinking != nil && *params.Thinking
		minBudget := int32(0) // 最低预算（不一定能完全关闭计算，但可以尽量减少/不输出）
		return append(opts, gemini.WithThinkingConfig(&genai.ThinkingConfig{
			IncludeThoughts: includeThoughts,
			ThinkingBudget:  &minBudget,
			// ThinkingLevel 留空（omitempty），避免部分模型报 “Thinking level is not supported”。
		}))
	}

	var includeThoughts bool
	var thinkingLevel genai.ThinkingLevel

	// nil 或 true 时启用思考
	if params.Thinking == nil || *params.Thinking {
		includeThoughts = true
		thinkingLevel = genai.ThinkingLevelHigh
	} else {
		includeThoughts = false
		thinkingLevel = genai.ThinkingLevelMinimal
	}

	return append(opts, gemini.WithThinkingConfig(&genai.ThinkingConfig{
		IncludeThoughts: includeThoughts,
		ThinkingLevel:   thinkingLevel,
	}))
}

// createSimulatedStreamReader 创建模拟流式 reader
// 将完整响应拆分发送，模拟流式体验
func createSimulatedStreamReader(resp *schema.Message) *schema.StreamReader[*schema.Message] {
	chunks := splitResponseToChunks(resp)
	return schema.StreamReaderFromArray(chunks)
}

// splitResponseToChunks 将完整响应拆分为多个 chunk
func splitResponseToChunks(resp *schema.Message) []*schema.Message {
	var chunks []*schema.Message

	// 1. 发送推理过程（逐段）
	if resp.ReasoningContent != "" {
		// 尝试按双换行符拆分段落
		paragraphs := strings.Split(resp.ReasoningContent, "\n\n")
		if len(paragraphs) <= 1 {
			// 没有段落分隔，按单换行符拆分
			paragraphs = strings.Split(resp.ReasoningContent, "\n")
		}

		for _, para := range paragraphs {
			para = strings.TrimSpace(para)
			if para != "" {
				chunks = append(chunks, &schema.Message{
					Role:             schema.Assistant,
					ReasoningContent: para + "\n",
				})
			}
		}
	}

	// 2. 发送文本内容（如果有）
	if resp.Content != "" {
		chunks = append(chunks, &schema.Message{
			Role:    schema.Assistant,
			Content: resp.Content,
		})
	}

	// 3. 发送多模态内容（整体）
	if len(resp.AssistantGenMultiContent) > 0 {
		chunks = append(chunks, &schema.Message{
			Role:                     schema.Assistant,
			AssistantGenMultiContent: resp.AssistantGenMultiContent,
		})
	}

	// 如果没有任何内容，返回空消息
	if len(chunks) == 0 {
		chunks = append(chunks, &schema.Message{
			Role: schema.Assistant,
		})
	}

	return chunks
}
