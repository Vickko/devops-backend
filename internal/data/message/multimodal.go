package message

import (
	"strings"

	"github.com/cloudwego/eino/schema"
)

// FilterMultimodalContent 根据模型能力过滤消息中的多模态内容
// 对不支持的模态内容使用占位符替代，而不是完全删除
func FilterMultimodalContent(messages []*schema.Message, clientName string) []*schema.Message {
	registry := GetModelCapabilityRegistry()
	caps := registry.GetCapabilities(clientName)

	// 如果模型不在注册表中，默认支持所有内容，直接返回
	if caps == nil {
		return messages
	}

	// 检查是否需要过滤
	needsFiltering := !caps.SupportedModalities[ModalityImage] ||
		!caps.SupportedModalities[ModalityAudio] ||
		!caps.SupportedModalities[ModalityVideo]

	if !needsFiltering {
		return messages
	}

	// 创建过滤后的消息副本
	filtered := make([]*schema.Message, len(messages))
	for i, msg := range messages {
		filtered[i] = &schema.Message{
			Role:       msg.Role,
			Content:    msg.Content,
			Name:       msg.Name,
			ToolCalls:  msg.ToolCalls,
			ToolCallID: msg.ToolCallID,
			ToolName:   msg.ToolName,
			Extra:      msg.Extra,
		}

		// 处理用户输入的多模态内容
		if len(msg.UserInputMultiContent) > 0 {
			filtered[i].UserInputMultiContent = filterInputMultiContent(
				msg.UserInputMultiContent,
				caps,
			)
		}

		// 处理助手生成的多模态内容
		if len(msg.AssistantGenMultiContent) > 0 {
			placeholders := buildContentPlaceholders(msg.AssistantGenMultiContent, caps)
			if len(placeholders) > 0 {
				// 将占位符追加到文本内容中
				if filtered[i].Content != "" {
					filtered[i].Content += "\n" + placeholders
				} else {
					filtered[i].Content = placeholders
				}
			}
		}

		// 推理内容通常是文本，如果模型不支持推理，也用占位符
		// 但大多数模型应该都支持文本，这里保守处理
		if msg.ReasoningContent != "" && !caps.SupportedModalities[ModalityText] {
			filtered[i].Content += "\n[Reasoning Content]"
		} else {
			filtered[i].ReasoningContent = msg.ReasoningContent
		}
	}

	return filtered
}

// filterInputMultiContent 过滤用户输入的多模态内容
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
				// 用文本占位符替代
				filtered = append(filtered, schema.MessageInputPart{
					Type: schema.ChatMessagePartTypeText,
					Text: "[Image]",
				})
			}
		case schema.ChatMessagePartTypeAudioURL:
			if caps.SupportedModalities[ModalityAudio] {
				filtered = append(filtered, part)
			} else {
				filtered = append(filtered, schema.MessageInputPart{
					Type: schema.ChatMessagePartTypeText,
					Text: "[Audio]",
				})
			}
		case schema.ChatMessagePartTypeVideoURL:
			if caps.SupportedModalities[ModalityVideo] {
				filtered = append(filtered, part)
			} else {
				filtered = append(filtered, schema.MessageInputPart{
					Type: schema.ChatMessagePartTypeText,
					Text: "[Video]",
				})
			}
		default:
			// 未知类型，保留
			filtered = append(filtered, part)
		}
	}

	return filtered
}

// buildContentPlaceholders 为助手生成的多模态内容构建占位符
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

	if len(placeholders) > 0 {
		return strings.Join(placeholders, " ")
	}

	return ""
}
