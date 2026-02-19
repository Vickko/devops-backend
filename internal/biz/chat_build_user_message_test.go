package biz

import (
	"testing"

	"github.com/cloudwego/eino/schema"
)

func TestBuildUserMessage_KeepContentForMixedMultimodalInput(t *testing.T) {
	req := &ChatRequest{
		Message: schema.Message{
			Role:    schema.User,
			Content: "请描述这张图",
			UserInputMultiContent: []schema.MessageInputPart{
				{Type: schema.ChatMessagePartTypeText, Text: "请描述这张图"},
				{
					Type: schema.ChatMessagePartTypeImageURL,
					Image: &schema.MessageInputImage{
						MessagePartCommon: schema.MessagePartCommon{MIMEType: "image/png"},
					},
				},
			},
		},
	}

	msg := BuildUserMessage(req)
	if msg == nil {
		t.Fatal("expected non-nil message")
	}
	if msg.Content != "请描述这张图" {
		t.Fatalf("unexpected content: %q", msg.Content)
	}
	if len(msg.UserInputMultiContent) != 2 {
		t.Fatalf("unexpected multimodal parts: %d", len(msg.UserInputMultiContent))
	}
}

