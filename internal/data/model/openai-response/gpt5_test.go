package openairesponse

import (
	"context"
	"errors"
	"fmt"
	"io"
	"testing"

	"github.com/cloudwego/eino/schema"
)

// TestGPT5StreamReasoning 测试 gpt-5 流式 reasoning.summary
func TestGPT5StreamReasoning(t *testing.T) {
	ctx := context.Background()

	client, err := NewChatModel(ctx, &Config{
		APIKey:  testAPIKey,
		BaseURL: testBaseURL,
		Model:   "gpt-5",
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

	stream, err := client.Stream(ctx, messages,
		WithReasoningEffort(ReasoningEffortHigh),
		WithReasoningSummary(ReasoningSummaryDetailed),
	)
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
			fmt.Printf("Content: %q\n", msg.Content)
		}
		if msg.ReasoningContent != "" {
			fullReasoning += msg.ReasoningContent
			fmt.Printf(">>> Reasoning: %q\n", msg.ReasoningContent)
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
		t.Log("⚠️ No streaming reasoning content")
	}
}
