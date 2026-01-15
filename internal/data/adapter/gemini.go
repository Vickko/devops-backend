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
	opts = a.injectThinkingConfig(opts)
	return a.raw.Generate(ctx, messages, opts...)
}

// Stream 流式调用，支持 fallback 到非流式模式
func (a *GeminiAdapter) Stream(ctx context.Context, messages []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	// 检查是否需要 fallback 到非流式模式
	registry := message.GetModelCapabilityRegistry()
	if registry.RequiresNonStreamingMode(a.modelName) {
		return a.streamWithFallback(ctx, messages, opts...)
	}

	opts = a.injectThinkingConfig(opts)
	return a.raw.Stream(ctx, messages, opts...)
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

// injectThinkingConfig 根据 opts 中的 RequestParams 注入 Gemini ThinkingConfig
func (a *GeminiAdapter) injectThinkingConfig(opts []model.Option) []model.Option {
	params := biz.GetParams(opts...)

	var includeThoughts bool
	var thinkingLevel genai.ThinkingLevel

	// nil 或 true 时启用思考
	if params.Thinking == nil || *params.Thinking {
		includeThoughts = true
		thinkingLevel = genai.ThinkingLevelHigh
	} else {
		includeThoughts = false
		thinkingLevel = genai.ThinkingLevelLow
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
