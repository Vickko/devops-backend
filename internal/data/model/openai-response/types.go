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

import "encoding/json"

// ReasoningEffort 推理努力程度
type ReasoningEffort string

const (
	ReasoningEffortLow    ReasoningEffort = "low"
	ReasoningEffortMedium ReasoningEffort = "medium"
	ReasoningEffortHigh   ReasoningEffort = "high"
)

// ReasoningSummary 推理摘要模式
type ReasoningSummary string

const (
	ReasoningSummaryAuto     ReasoningSummary = "auto"
	ReasoningSummaryConcise  ReasoningSummary = "concise"
	ReasoningSummaryDetailed ReasoningSummary = "detailed"
)

// ResponsesRequest Responses API 请求结构
type ResponsesRequest struct {
	Model              string           `json:"model"`
	Input              any              `json:"input"` // string 或 []InputItem
	Instructions       string           `json:"instructions,omitempty"`
	MaxOutputTokens    *int             `json:"max_output_tokens,omitempty"`
	Temperature        *float64         `json:"temperature,omitempty"`
	TopP               *float64         `json:"top_p,omitempty"`
	Reasoning          *ReasoningConfig `json:"reasoning,omitempty"`
	Store              *bool            `json:"store,omitempty"`
	Stream             bool             `json:"stream,omitempty"`
	PreviousResponseID string           `json:"previous_response_id,omitempty"`
	Tools              []FunctionTool   `json:"tools,omitempty"`
	ToolChoice         any              `json:"tool_choice,omitempty"`
	ParallelToolCalls  *bool            `json:"parallel_tool_calls,omitempty"`
}

// ReasoningConfig 推理配置
type ReasoningConfig struct {
	Effort  string `json:"effort,omitempty"`
	Summary string `json:"summary,omitempty"`
}

// InputItem 输入项
type InputItem struct {
	Type    string `json:"type,omitempty"` // message, function_call_output
	Role    string `json:"role,omitempty"`
	Content any    `json:"content,omitempty"` // string 或 []ContentItem

	// function_call_output 类型使用
	CallID string `json:"call_id,omitempty"`
	Output string `json:"output,omitempty"`
}

// ContentItem 内容项
type ContentItem struct {
	Type     string    `json:"type"` // input_text, input_image, input_file
	Text     string    `json:"text,omitempty"`
	ImageURL *ImageURL `json:"image_url,omitempty"`
	FileURL  *FileURL  `json:"file_url,omitempty"`
}

// ImageURL 图片 URL
type ImageURL struct {
	URL    string `json:"url"`
	Detail string `json:"detail,omitempty"` // auto, low, high
}

// FileURL 文件 URL
type FileURL struct {
	URL string `json:"url"`
}

// FunctionTool 函数工具定义
type FunctionTool struct {
	Type     string              `json:"type"` // function
	Function *FunctionDefinition `json:"function,omitempty"`
}

// FunctionDefinition 函数定义
type FunctionDefinition struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Parameters  any    `json:"parameters,omitempty"`
	Strict      *bool  `json:"strict,omitempty"`
}

// ResponsesResponse Responses API 响应结构
type ResponsesResponse struct {
	ID                string             `json:"id"`
	Object            string             `json:"object"`
	CreatedAt         int64              `json:"created_at"`
	Status            string             `json:"status"` // completed, failed, in_progress, cancelled, queued, incomplete
	Output            []OutputItem       `json:"output"`
	Usage             *Usage             `json:"usage,omitempty"`
	Error             *ErrorInfo         `json:"error,omitempty"`
	IncompleteDetails *IncompleteDetails `json:"incomplete_details,omitempty"`
	Model             string             `json:"model,omitempty"`
}

// OutputItem 输出项
type OutputItem struct {
	Type    string          `json:"type"` // message, reasoning, function_call
	ID      string          `json:"id,omitempty"`
	Role    string          `json:"role,omitempty"`
	Status  string          `json:"status,omitempty"`
	Content []OutputContent `json:"content,omitempty"`
	Summary []SummaryItem   `json:"summary,omitempty"` // for reasoning type

	// function_call 类型使用
	CallID    string `json:"call_id,omitempty"`
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
}

// OutputContent 输出内容
type OutputContent struct {
	Type        string       `json:"type"` // output_text
	Text        string       `json:"text,omitempty"`
	Annotations []Annotation `json:"annotations,omitempty"`
}

// Annotation 注释
type Annotation struct {
	Type string `json:"type"`
	// 可扩展其他注释类型字段
}

// SummaryItem 摘要项 (用于 reasoning 输出)
type SummaryItem struct {
	Type string `json:"type"` // summary_text
	Text string `json:"text"`
}

// Usage token 使用统计
type Usage struct {
	InputTokens        int                 `json:"input_tokens"`
	InputTokensDetails *InputTokensDetails `json:"input_tokens_details,omitempty"`
	OutputTokens       int                 `json:"output_tokens"`
	OutputTokensDetails *OutputTokensDetails `json:"output_tokens_details,omitempty"`
	TotalTokens        int                 `json:"total_tokens"`
}

// InputTokensDetails 输入 token 详情
type InputTokensDetails struct {
	CachedTokens int `json:"cached_tokens"`
}

// OutputTokensDetails 输出 token 详情
type OutputTokensDetails struct {
	ReasoningTokens int `json:"reasoning_tokens"`
}

// ErrorInfo 错误信息
type ErrorInfo struct {
	Code    string `json:"code,omitempty"`
	Message string `json:"message"`
}

// IncompleteDetails 不完整详情
type IncompleteDetails struct {
	Reason string `json:"reason,omitempty"` // max_output_tokens, etc.
}

// StreamEvent 流式事件
type StreamEvent struct {
	Event string          `json:"event"`
	Data  json.RawMessage `json:"data"`
}

// StreamResponseCreated response.created 事件数据
type StreamResponseCreated struct {
	Response *ResponsesResponse `json:"response"`
}

// StreamOutputItemAdded response.output_item.added 事件数据
type StreamOutputItemAdded struct {
	OutputIndex int         `json:"output_index"`
	Item        *OutputItem `json:"item"`
}

// StreamContentPartAdded response.content_part.added 事件数据
type StreamContentPartAdded struct {
	OutputIndex  int            `json:"output_index"`
	ContentIndex int            `json:"content_index"`
	Part         *OutputContent `json:"part"`
}

// StreamOutputTextDelta response.output_text.delta 事件数据
type StreamOutputTextDelta struct {
	OutputIndex  int    `json:"output_index"`
	ContentIndex int    `json:"content_index"`
	Delta        string `json:"delta"`
}

// StreamReasoningSummaryTextDelta response.reasoning_summary_text.delta 事件数据
type StreamReasoningSummaryTextDelta struct {
	OutputIndex  int    `json:"output_index"`
	SummaryIndex int    `json:"summary_index"`
	Delta        string `json:"delta"`
}

// StreamFunctionCallArgumentsDelta response.function_call_arguments.delta 事件数据
type StreamFunctionCallArgumentsDelta struct {
	OutputIndex int    `json:"output_index"`
	CallID      string `json:"call_id"`
	Delta       string `json:"delta"`
}

// StreamOutputItemDone response.output_item.done 事件数据
type StreamOutputItemDone struct {
	OutputIndex int         `json:"output_index"`
	Item        *OutputItem `json:"item"`
}

// StreamResponseCompleted response.completed 事件数据
type StreamResponseCompleted struct {
	Response *ResponsesResponse `json:"response"`
}

// StreamResponseFailed response.failed 事件数据
type StreamResponseFailed struct {
	Response *ResponsesResponse `json:"response"`
}

// StreamError error 事件数据
type StreamError struct {
	Code    string `json:"code,omitempty"`
	Message string `json:"message"`
}
