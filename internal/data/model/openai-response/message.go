/*
 * Copyright 2024 DevOps Backend Authors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package openairesponse

import (
	"fmt"
	"strings"

	"github.com/cloudwego/eino/schema"
)

// toResponsesInput 将 schema.Message 列表转换为 Responses API 输入格式
// 返回 input 数组和提取的 instructions (从 system 消息)
func toResponsesInput(messages []*schema.Message) ([]InputItem, string, error) {
	var input []InputItem
	var instructions string

	for _, msg := range messages {
		switch msg.Role {
		case schema.System:
			// System 消息提取为 instructions
			if instructions != "" {
				instructions += "\n\n"
			}
			instructions += msg.Content
		case schema.User:
			item, err := toUserInputItem(msg)
			if err != nil {
				return nil, "", fmt.Errorf("convert user message: %w", err)
			}
			input = append(input, item)
		case schema.Assistant:
			item, err := toAssistantInputItem(msg)
			if err != nil {
				return nil, "", fmt.Errorf("convert assistant message: %w", err)
			}
			input = append(input, item)
		case schema.Tool:
			item := toToolOutputItem(msg)
			input = append(input, item)
		default:
			return nil, "", fmt.Errorf("unsupported role: %s", msg.Role)
		}
	}

	return input, instructions, nil
}

// toUserInputItem 转换用户消息
func toUserInputItem(msg *schema.Message) (InputItem, error) {
	item := InputItem{
		Type: "message",
		Role: "user",
	}

	// 处理多模态内容
	if len(msg.UserInputMultiContent) > 0 {
		contents, err := toUserMultiContent(msg.UserInputMultiContent)
		if err != nil {
			return item, err
		}
		item.Content = contents
		return item, nil
	}

	// 处理旧版多模态内容
	if len(msg.MultiContent) > 0 {
		contents, err := toMultiContent(msg.MultiContent)
		if err != nil {
			return item, err
		}
		item.Content = contents
		return item, nil
	}

	// 纯文本内容
	if msg.Content != "" {
		item.Content = msg.Content
	}

	return item, nil
}

// toAssistantInputItem 转换助手消息
func toAssistantInputItem(msg *schema.Message) (InputItem, error) {
	item := InputItem{
		Type: "message",
		Role: "assistant",
	}

	// 处理多模态输出内容
	if len(msg.AssistantGenMultiContent) > 0 {
		var contents []ContentItem
		for _, part := range msg.AssistantGenMultiContent {
			if part.Type == schema.ChatMessagePartTypeText {
				contents = append(contents, ContentItem{
					Type: "input_text",
					Text: part.Text,
				})
			}
		}
		if len(contents) > 0 {
			item.Content = contents
			return item, nil
		}
	}

	// 纯文本内容
	if msg.Content != "" {
		item.Content = msg.Content
	}

	return item, nil
}

// toToolOutputItem 转换工具输出消息
func toToolOutputItem(msg *schema.Message) InputItem {
	return InputItem{
		Type:   "function_call_output",
		CallID: msg.ToolCallID,
		Output: msg.Content,
	}
}

// toUserMultiContent 转换用户多模态内容
func toUserMultiContent(parts []schema.MessageInputPart) ([]ContentItem, error) {
	var contents []ContentItem

	for _, part := range parts {
		switch part.Type {
		case schema.ChatMessagePartTypeText:
			contents = append(contents, ContentItem{
				Type: "input_text",
				Text: part.Text,
			})
		case schema.ChatMessagePartTypeImageURL:
			if part.Image == nil {
				return nil, fmt.Errorf("image field is required for image_url type")
			}
			url, err := getImageURL(part.Image)
			if err != nil {
				return nil, err
			}
			contents = append(contents, ContentItem{
				Type: "input_image",
				ImageURL: &ImageURL{
					URL:    url,
					Detail: string(part.Image.Detail),
				},
			})
		case schema.ChatMessagePartTypeFileURL:
			if part.File == nil {
				return nil, fmt.Errorf("file field is required for file_url type")
			}
			url, err := getFileURL(part.File)
			if err != nil {
				return nil, err
			}
			contents = append(contents, ContentItem{
				Type: "input_file",
				FileURL: &FileURL{
					URL: url,
				},
			})
		default:
			return nil, fmt.Errorf("unsupported content type: %s", part.Type)
		}
	}

	return contents, nil
}

// toMultiContent 转换旧版多模态内容
func toMultiContent(parts []schema.ChatMessagePart) ([]ContentItem, error) {
	var contents []ContentItem

	for _, part := range parts {
		switch part.Type {
		case schema.ChatMessagePartTypeText:
			contents = append(contents, ContentItem{
				Type: "input_text",
				Text: part.Text,
			})
		case schema.ChatMessagePartTypeImageURL:
			if part.ImageURL == nil {
				return nil, fmt.Errorf("ImageURL field is required for image_url type")
			}
			contents = append(contents, ContentItem{
				Type: "input_image",
				ImageURL: &ImageURL{
					URL:    part.ImageURL.URL,
					Detail: string(part.ImageURL.Detail),
				},
			})
		default:
			return nil, fmt.Errorf("unsupported content type: %s", part.Type)
		}
	}

	return contents, nil
}

// getImageURL 从 MessageInputImage 获取 URL
func getImageURL(img *schema.MessageInputImage) (string, error) {
	if img.URL != nil {
		return *img.URL, nil
	}
	if img.Base64Data != nil {
		if img.MIMEType == "" {
			return "", fmt.Errorf("MIMEType is required when using Base64Data")
		}
		return fmt.Sprintf("data:%s;base64,%s", img.MIMEType, *img.Base64Data), nil
	}
	return "", fmt.Errorf("either URL or Base64Data is required for image")
}

// getFileURL 从 MessageInputFile 获取 URL
func getFileURL(file *schema.MessageInputFile) (string, error) {
	if file.URL != nil {
		return *file.URL, nil
	}
	if file.Base64Data != nil {
		if file.MIMEType == "" {
			return "", fmt.Errorf("MIMEType is required when using Base64Data")
		}
		return fmt.Sprintf("data:%s;base64,%s", file.MIMEType, *file.Base64Data), nil
	}
	return "", fmt.Errorf("either URL or Base64Data is required for file")
}

// toSchemaMessage 将 Responses API 输出转换为 schema.Message
func toSchemaMessage(output []OutputItem, usage *Usage) *schema.Message {
	msg := &schema.Message{
		Role: schema.Assistant,
	}

	var reasoningParts []string
	var contentParts []string
	var toolCalls []schema.ToolCall

	for _, item := range output {
		switch item.Type {
		case "reasoning":
			// 提取推理摘要
			for _, summary := range item.Summary {
				if summary.Type == "summary_text" && summary.Text != "" {
					reasoningParts = append(reasoningParts, summary.Text)
				}
			}
		case "message":
			// 提取消息内容
			for _, content := range item.Content {
				if content.Type == "output_text" && content.Text != "" {
					contentParts = append(contentParts, content.Text)
				}
			}
		case "function_call":
			// 提取函数调用
			toolCalls = append(toolCalls, schema.ToolCall{
				ID:   item.CallID,
				Type: "function",
				Function: schema.FunctionCall{
					Name:      item.Name,
					Arguments: item.Arguments,
				},
			})
		}
	}

	// 设置推理内容
	if len(reasoningParts) > 0 {
		msg.ReasoningContent = strings.Join(reasoningParts, "\n\n")
	}

	// 设置消息内容
	if len(contentParts) > 0 {
		msg.Content = strings.Join(contentParts, "")
	}

	// 设置工具调用
	if len(toolCalls) > 0 {
		msg.ToolCalls = toolCalls
	}

	// 设置 Usage
	if usage != nil {
		msg.ResponseMeta = &schema.ResponseMeta{
			Usage: toSchemaTokenUsage(usage),
		}
	}

	return msg
}

// toSchemaTokenUsage 转换 token 使用统计
func toSchemaTokenUsage(usage *Usage) *schema.TokenUsage {
	if usage == nil {
		return nil
	}

	tokenUsage := &schema.TokenUsage{
		PromptTokens:     usage.InputTokens,
		CompletionTokens: usage.OutputTokens,
		TotalTokens:      usage.TotalTokens,
	}

	if usage.InputTokensDetails != nil {
		tokenUsage.PromptTokenDetails.CachedTokens = usage.InputTokensDetails.CachedTokens
	}

	if usage.OutputTokensDetails != nil {
		tokenUsage.CompletionTokensDetails.ReasoningTokens = usage.OutputTokensDetails.ReasoningTokens
	}

	return tokenUsage
}

// toTools 将 schema.ToolInfo 转换为 FunctionTool
func toTools(tools []*schema.ToolInfo) ([]FunctionTool, error) {
	if len(tools) == 0 {
		return nil, nil
	}

	result := make([]FunctionTool, len(tools))
	for i, tool := range tools {
		if tool == nil {
			return nil, fmt.Errorf("tool info cannot be nil")
		}

		params, err := tool.ParamsOneOf.ToJSONSchema()
		if err != nil {
			return nil, fmt.Errorf("convert tool parameters: %w", err)
		}

		result[i] = FunctionTool{
			Type: "function",
			Function: &FunctionDefinition{
				Name:        tool.Name,
				Description: tool.Desc,
				Parameters:  params,
			},
		}
	}

	return result, nil
}
