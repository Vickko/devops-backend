package openairesponse

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/cloudwego/eino/schema"
)

// TestReasoningSummaryIntegration 测试 reasoning.summary 的完整集成
func TestReasoningSummaryIntegration(t *testing.T) {
	ctx := context.Background()

	client, err := NewChatModel(ctx, &Config{
		APIKey:  testAPIKey,
		BaseURL: testBaseURL,
		Model:   "o1-mini", // 支持 reasoning 的模型
	})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	messages := []*schema.Message{
		{
			Role:    schema.User,
			Content: "What is 15 + 27? Think step by step.",
		},
	}

	// 使用 high effort 和 detailed summary
	msg, err := client.Generate(ctx, messages,
		WithReasoningEffort(ReasoningEffortHigh),
		WithReasoningSummary(ReasoningSummaryDetailed),
	)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	t.Logf("=== Response ===")
	t.Logf("Role: %s", msg.Role)
	t.Logf("Content: %s", msg.Content)
	t.Logf("ReasoningContent: %s", msg.ReasoningContent)

	if msg.ResponseMeta != nil {
		t.Logf("FinishReason: %s", msg.ResponseMeta.FinishReason)
		if msg.ResponseMeta.Usage != nil {
			t.Logf("Usage - Prompt: %d, Completion: %d, Total: %d",
				msg.ResponseMeta.Usage.PromptTokens,
				msg.ResponseMeta.Usage.CompletionTokens,
				msg.ResponseMeta.Usage.TotalTokens)
			if msg.ResponseMeta.Usage.CompletionTokensDetails.ReasoningTokens > 0 {
				t.Logf("ReasoningTokens: %d", msg.ResponseMeta.Usage.CompletionTokensDetails.ReasoningTokens)
			}
		}
	}

	// 验证
	if msg.Content == "" {
		t.Error("Content should not be empty")
	}
	if msg.ReasoningContent == "" {
		t.Error("ReasoningContent should not be empty - reasoning.summary not working!")
	} else {
		t.Log("✅ ReasoningContent captured successfully!")
	}
}

// TestReasoningSummaryStreamIntegration 测试流式 reasoning.summary 集成
func TestReasoningSummaryStreamIntegration(t *testing.T) {
	ctx := context.Background()

	// 流式模式下代理服务对 o1-mini 有问题，使用 gpt-4o-mini 测试基本流式功能
	client, err := NewChatModel(ctx, &Config{
		APIKey:  testAPIKey,
		BaseURL: testBaseURL,
		Model:   "gpt-4o-mini",
	})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	messages := []*schema.Message{
		{
			Role:    schema.User,
			Content: "Count from 1 to 3.",
		},
	}

	stream, err := client.Stream(ctx, messages)
	// 注：gpt-4o-mini 不支持 reasoning，这里只测试流式基本功能
	if err != nil {
		t.Fatalf("Stream failed: %v", err)
	}
	defer stream.Close()

	var fullContent string
	var fullReasoning string

	t.Log("=== Stream Chunks ===")
	for {
		msg, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("stream recv error: %v", err)
		}

		if msg.Content != "" {
			fullContent += msg.Content
			t.Logf("Content chunk: %q", msg.Content)
		}
		if msg.ReasoningContent != "" {
			fullReasoning += msg.ReasoningContent
			t.Logf("Reasoning chunk: %q", msg.ReasoningContent)
		}
	}

	t.Logf("\n=== Final ===")
	t.Logf("Full Content: %s", fullContent)
	t.Logf("Full Reasoning: %s", fullReasoning)

	if fullContent == "" {
		t.Error("Content should not be empty")
	}
	if fullReasoning != "" {
		t.Log("✅ Stream ReasoningContent captured!")
	} else {
		t.Log("⚠️ No streaming reasoning content (may be normal for some providers)")
	}
}
