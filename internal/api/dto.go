package api

import (
	"context"
	"encoding/json"
	"time"

	"github.com/cloudwego/eino/schema"
)

// RunAgentInput AG-UI 运行请求
type RunAgentInput struct {
	ThreadID       string                 `json:"threadId,omitempty"`
	RunID          string                 `json:"runId,omitempty"`
	State          map[string]any         `json:"state,omitempty"`
	Messages       []RunAgentInputMessage `json:"messages"`
	Tools          []json.RawMessage      `json:"tools,omitempty"`
	Context        []json.RawMessage      `json:"context,omitempty"`
	ForwardedProps map[string]any         `json:"forwardedProps,omitempty"`
}

// RunAgentInputMessage AG-UI 消息
type RunAgentInputMessage struct {
	ID         string            `json:"id,omitempty"`
	Role       string            `json:"role"`
	Content    json.RawMessage   `json:"content"`
	Name       string            `json:"name,omitempty"`
	ToolCallID string            `json:"toolCallId,omitempty"`
	ToolCalls  []schema.ToolCall `json:"toolCalls,omitempty"`
}

// RunAgentInputContentPart AG-UI 消息内容分片（当前仅解析 text）
type RunAgentInputContentPart struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	Data     string `json:"data,omitempty"`
	MimeType string `json:"mimeType,omitempty"`
}

// ChatRequest 内部聊天请求 DTO
type ChatRequest struct {
	schema.Message
	Model    string `json:"-"`
	ThreadID string `json:"-"`
	RunID    string `json:"-"`
	Thinking *bool  `json:"-"`
}

// ChatResponse 聊天响应 DTO
type ChatResponse struct {
	schema.Message
	Model     string `json:"model,omitempty"`
	SessionID string `json:"session,omitempty"`
	TreeID    string `json:"tree_id,omitempty"`
}

// StreamChunk 流式响应块
type StreamChunk struct {
	Content                  string                     `json:"content,omitempty"`
	ReasoningContent         string                     `json:"reasoning_content,omitempty"`
	AssistantGenMultiContent []schema.MessageOutputPart `json:"assistant_gen_multi_content,omitempty"`
	ToolCalls                []schema.ToolCall          `json:"tool_calls,omitempty"`
}

// StreamMetaInfo 流开始时的元信息
type StreamMetaInfo struct {
	ThreadID  string `json:"threadId"`
	SessionID string `json:"sessionId"`
	RunID     string `json:"runId"`
	IsNew     bool   `json:"isNew"`
}

// StreamStartCallback 流开始时的回调，传递元信息
type StreamStartCallback func(info StreamMetaInfo) error

// StreamChunkCallback 流数据回调
type StreamChunkCallback func(chunk StreamChunk) error

// SessionInfo 会话树信息 DTO（对外展示）
type SessionInfo struct {
	ID                  string    `json:"id"`
	Title               string    `json:"title"`
	LastActiveSessionID string    `json:"last_active_session_id"`
	LastMessage         string    `json:"last_message"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

// ListSessionsResponse 会话列表响应
type ListSessionsResponse struct {
	Sessions []SessionInfo `json:"sessions"`
}

// GetSessionResponse 获取会话详情响应
type GetSessionResponse struct {
	Messages []*ChatResponse `json:"messages"`
}

// ChatService 聊天服务接口（由 service 层实现）
type ChatService interface {
	Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error)
	ChatStream(
		ctx context.Context,
		req *ChatRequest,
		onStart StreamStartCallback,
		onChunk StreamChunkCallback,
	) error
	ListSessions(ctx context.Context) ([]SessionInfo, error)
	GetSession(ctx context.Context, sessionID string) (*GetSessionResponse, error)
}
