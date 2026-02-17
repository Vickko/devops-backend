package provider

import (
	"strings"

	"github.com/cloudwego/eino/schema"
)

// FilterMultimodalContent 根据模型能力过滤消息中的多模态内容
func FilterMultimodalContent(messages []*schema.Message, clientName string) []*schema.Message {
	registry := GetModelCapabilityRegistry()
	caps := registry.GetCapabilities(clientName)
	if caps == nil {
		return messages
	}

	needsFiltering := !caps.SupportedModalities[ModalityImage] ||
		!caps.SupportedModalities[ModalityAudio] ||
		!caps.SupportedModalities[ModalityVideo]
	if !needsFiltering {
		return messages
	}

	filtered := make([]*schema.Message, len(messages))
	for i, msg := range messages {
		filtered[i] = &schema.Message{
			Role: msg.Role, Content: msg.Content, Name: msg.Name,
			ToolCalls: msg.ToolCalls, ToolCallID: msg.ToolCallID,
			ToolName: msg.ToolName, Extra: msg.Extra,
		}
		if len(msg.UserInputMultiContent) > 0 {
			filtered[i].UserInputMultiContent = filterInputMultiContent(msg.UserInputMultiContent, caps)
		}
		if len(msg.AssistantGenMultiContent) > 0 {
			ph := buildContentPlaceholders(msg.AssistantGenMultiContent, caps)
			if ph != "" {
				if filtered[i].Content != "" {
					filtered[i].Content += "\n" + ph
				} else {
					filtered[i].Content = ph
				}
			}
		}
		if msg.ReasoningContent != "" && !caps.SupportedModalities[ModalityText] {
			filtered[i].Content += "\n[Reasoning Content]"
		} else {
			filtered[i].ReasoningContent = msg.ReasoningContent
		}
	}
	return filtered
}

func filterInputMultiContent(parts []schema.MessageInputPart, caps *ModelCapabilities) []schema.MessageInputPart {
	var filtered []schema.MessageInputPart
	for _, part := range parts {
		switch part.Type {
		case schema.ChatMessagePartTypeText:
			if caps.SupportedModalities[ModalityText] {
				filtered = append(filtered, part)
			}
		case schema.ChatMessagePartTypeImageURL:
			if caps.SupportedModalities[ModalityImage] {
				filtered = append(filtered, part)
			} else {
				filtered = append(filtered, schema.MessageInputPart{Type: schema.ChatMessagePartTypeText, Text: "[Image]"})
			}
		case schema.ChatMessagePartTypeAudioURL:
			if caps.SupportedModalities[ModalityAudio] {
				filtered = append(filtered, part)
			} else {
				filtered = append(filtered, schema.MessageInputPart{Type: schema.ChatMessagePartTypeText, Text: "[Audio]"})
			}
		case schema.ChatMessagePartTypeVideoURL:
			if caps.SupportedModalities[ModalityVideo] {
				filtered = append(filtered, part)
			} else {
				filtered = append(filtered, schema.MessageInputPart{Type: schema.ChatMessagePartTypeText, Text: "[Video]"})
			}
		default:
			filtered = append(filtered, part)
		}
	}
	return filtered
}

func buildContentPlaceholders(parts []schema.MessageOutputPart, caps *ModelCapabilities) string {
	var placeholders []string
	for _, part := range parts {
		switch part.Type {
		case schema.ChatMessagePartTypeImageURL:
			if !caps.SupportedModalities[ModalityImage] {
				placeholders = append(placeholders, "[Image]")
			}
		case schema.ChatMessagePartTypeAudioURL:
			if !caps.SupportedModalities[ModalityAudio] {
				placeholders = append(placeholders, "[Audio]")
			}
		case schema.ChatMessagePartTypeVideoURL:
			if !caps.SupportedModalities[ModalityVideo] {
				placeholders = append(placeholders, "[Video]")
			}
		}
	}
	return strings.Join(placeholders, " ")
}
