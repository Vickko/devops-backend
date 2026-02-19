package biz

import (
	"testing"

	"github.com/cloudwego/eino/schema"
)

func TestPrepareMessagesForModel_UserMixedContent(t *testing.T) {
	orig := &schema.Message{
		Role:    schema.User,
		Content: "look",
		UserInputMultiContent: []schema.MessageInputPart{
			{Type: schema.ChatMessagePartTypeText, Text: "look"},
			{
				Type: schema.ChatMessagePartTypeImageURL,
				Image: &schema.MessageInputImage{
					MessagePartCommon: schema.MessagePartCommon{MIMEType: "image/png"},
				},
			},
		},
	}

	got := prepareMessagesForModel([]*schema.Message{orig})
	if got[0].Content != "" {
		t.Fatalf("expected empty content for mixed multimodal message, got: %q", got[0].Content)
	}
	if len(got[0].UserInputMultiContent) != 2 {
		t.Fatalf("unexpected user multimodal parts: %d", len(got[0].UserInputMultiContent))
	}
	if orig.Content != "look" {
		t.Fatalf("prepareMessagesForModel should not mutate source message")
	}
}

func TestPrepareMessagesForModel_UserTextOnlyKeepsContent(t *testing.T) {
	got := prepareMessagesForModel([]*schema.Message{
		{
			Role:    schema.User,
			Content: "hello",
			UserInputMultiContent: []schema.MessageInputPart{
				{Type: schema.ChatMessagePartTypeText, Text: "hello"},
			},
		},
	})

	if got[0].Content != "hello" {
		t.Fatalf("unexpected content: %q", got[0].Content)
	}
	if len(got[0].UserInputMultiContent) != 0 {
		t.Fatalf("expected user multimodal content to be cleared")
	}
}

func TestPrepareMessagesForModel_UserTextOnlyBackfillsContent(t *testing.T) {
	got := prepareMessagesForModel([]*schema.Message{
		{
			Role: schema.User,
			UserInputMultiContent: []schema.MessageInputPart{
				{Type: schema.ChatMessagePartTypeText, Text: "hello "},
				{Type: schema.ChatMessagePartTypeText, Text: "world"},
			},
		},
	})

	if got[0].Content != "hello world" {
		t.Fatalf("unexpected merged content: %q", got[0].Content)
	}
	if len(got[0].UserInputMultiContent) != 0 {
		t.Fatalf("expected user multimodal content to be cleared")
	}
}

func TestPrepareMessagesForModel_NonUserClearsUserMultiContent(t *testing.T) {
	got := prepareMessagesForModel([]*schema.Message{
		{
			Role: schema.Assistant,
			UserInputMultiContent: []schema.MessageInputPart{
				{Type: schema.ChatMessagePartTypeText, Text: "legacy"},
			},
		},
	})

	if got[0].Content != "legacy" {
		t.Fatalf("unexpected fallback content: %q", got[0].Content)
	}
	if len(got[0].UserInputMultiContent) != 0 {
		t.Fatalf("expected user multimodal content to be cleared for non-user roles")
	}
}
