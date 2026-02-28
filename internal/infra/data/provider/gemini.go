package provider

import (
	"context"
	"fmt"
	"strings"

	"devops-backend/internal/biz"
	"devops-backend/internal/conf"

	"github.com/cloudwego/eino-ext/components/model/gemini"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"google.golang.org/genai"
)

// newGeminiRaw 创建原始 Gemini client（忠实反映厂商默认行为）
func newGeminiRaw(ctx context.Context, cfg conf.Client, modelName string, opts ...model.Option) (model.ToolCallingChatModel, error) {
	gc, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:      cfg.APIKey,
		HTTPOptions: genai.HTTPOptions{BaseURL: cfg.BaseURL},
	})
	if err != nil {
		return nil, fmt.Errorf("create gemini client: %w", err)
	}
	return gemini.NewChatModel(ctx, &gemini.Config{
		Client: gc, Model: modelName,
	})
}

// newGemini 创建 Gemini 模型 + thinking/fallback adapter
func newGemini(ctx context.Context, cfg conf.Client, modelName string, opts ...model.Option) (model.ToolCallingChatModel, error) {
	gc, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:      cfg.APIKey,
		HTTPOptions: genai.HTTPOptions{BaseURL: cfg.BaseURL},
	})
	if err != nil {
		return nil, fmt.Errorf("create gemini client: %w", err)
	}
	raw, err := gemini.NewChatModel(ctx, &gemini.Config{
		Client: gc, Model: modelName,
		ResponseModalities: []gemini.GeminiResponseModality{
			gemini.GeminiResponseModalityText,
			gemini.GeminiResponseModalityImage,
		},
	})
	if err != nil {
		return nil, err
	}
	return &geminiAdapter{raw: raw, modelName: modelName}, nil
}

type geminiAdapter struct {
	raw       model.ToolCallingChatModel
	modelName string
}

func (a *geminiAdapter) GetType() string {
	if c, ok := a.raw.(interface{ GetType() string }); ok {
		return c.GetType()
	}
	return "Gemini"
}

func (a *geminiAdapter) IsCallbacksEnabled() bool {
	if c, ok := a.raw.(interface{ IsCallbacksEnabled() bool }); ok {
		return c.IsCallbacksEnabled()
	}
	return true
}

func (a *geminiAdapter) Generate(ctx context.Context, messages []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	params := biz.GetParams(opts...)
	resp, err := a.raw.Generate(ctx, messages, a.injectThinkingConfig(opts)...)
	if err != nil {
		return nil, err
	}
	if params.Thinking != nil && !*params.Thinking {
		resp.ReasoningContent = ""
	}
	return resp, nil
}

func (a *geminiAdapter) Stream(ctx context.Context, messages []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	params := biz.GetParams(opts...)
	if GetModelCapabilityRegistry().RequiresNonStreamingMode(a.modelName) {
		resp, err := a.raw.Generate(ctx, messages, a.injectThinkingConfig(opts)...)
		if err != nil {
			return nil, err
		}
		return wrapHideThinking(createSimulatedStreamReader(resp), params), nil
	}
	sr, err := a.raw.Stream(ctx, messages, a.injectThinkingConfig(opts)...)
	if err != nil {
		return nil, err
	}
	return wrapHideThinking(sr, params), nil
}

func (a *geminiAdapter) WithTools(tools []*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	m, err := a.raw.WithTools(tools)
	if err != nil {
		return nil, err
	}
	return &geminiAdapter{raw: m, modelName: a.modelName}, nil
}

func (a *geminiAdapter) injectThinkingConfig(opts []model.Option) []model.Option {
	params := biz.GetParams(opts...)
	// adapter 负责设置所有可能的 response modalities
	opts = append(opts, gemini.WithResponseModalities([]gemini.GeminiResponseModality{
		gemini.GeminiResponseModalityText,
		gemini.GeminiResponseModalityImage,
		gemini.GeminiResponseModalityAudio,
	}))
	if GetModelCapabilityRegistry().RequiresNonStreamingMode(a.modelName) {
		include := params.Thinking != nil && *params.Thinking
		budget := int32(0)
		return append(opts, gemini.WithThinkingConfig(&genai.ThinkingConfig{
			IncludeThoughts: include, ThinkingBudget: &budget,
		}))
	}
	var include bool
	var level genai.ThinkingLevel
	if params.Thinking == nil || *params.Thinking {
		include, level = true, genai.ThinkingLevelHigh
	} else {
		include, level = false, genai.ThinkingLevelMinimal
	}
	return append(opts, gemini.WithThinkingConfig(&genai.ThinkingConfig{
		IncludeThoughts: include, ThinkingLevel: level,
	}))
}

func wrapHideThinking(sr *schema.StreamReader[*schema.Message], params *biz.RequestParams) *schema.StreamReader[*schema.Message] {
	if params == nil || params.Thinking == nil || *params.Thinking {
		return sr
	}
	return schema.StreamReaderWithConvert(sr, func(m *schema.Message) (*schema.Message, error) {
		if m == nil || m.ReasoningContent == "" {
			return m, nil
		}
		out := *m
		out.ReasoningContent = ""
		if out.Content == "" && len(out.AssistantGenMultiContent) == 0 {
			return nil, schema.ErrNoValue
		}
		return &out, nil
	})
}

func createSimulatedStreamReader(resp *schema.Message) *schema.StreamReader[*schema.Message] {
	return schema.StreamReaderFromArray(splitResponseToChunks(resp))
}

func splitResponseToChunks(resp *schema.Message) []*schema.Message {
	var chunks []*schema.Message
	if resp.ReasoningContent != "" {
		paras := strings.Split(resp.ReasoningContent, "\n\n")
		if len(paras) <= 1 {
			paras = strings.Split(resp.ReasoningContent, "\n")
		}
		for _, p := range paras {
			if p = strings.TrimSpace(p); p != "" {
				chunks = append(chunks, &schema.Message{Role: schema.Assistant, ReasoningContent: p + "\n"})
			}
		}
	}
	if resp.Content != "" {
		chunks = append(chunks, &schema.Message{Role: schema.Assistant, Content: resp.Content})
	}
	if len(resp.AssistantGenMultiContent) > 0 {
		chunks = append(chunks, &schema.Message{Role: schema.Assistant, AssistantGenMultiContent: resp.AssistantGenMultiContent})
	}
	if len(chunks) == 0 {
		chunks = []*schema.Message{{Role: schema.Assistant}}
	}
	return chunks
}
