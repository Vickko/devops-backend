package api

import (
	"context"
	"time"

	"github.com/cloudwego/eino/schema"
)

// ChatRequest 聊天请求 DTO
type ChatRequest struct {
	schema.Message
	Model    string `json:"model,omitempty"`
	Client   string `json:"client,omitempty"` // 指定客户端，可覆盖关键词匹配
	Session  string `json:"session,omitempty"` // 可选，不传时后端生成
	Thinking *bool  `json:"thinking,omitempty"` // 是否启用思考模式
}

// ChatResponse 聊天响应 DTO
type ChatResponse struct {
	schema.Message
	Model string `json:"model,omitempty"`
}

// StreamChunk 流式响应块
type StreamChunk struct {
	Content                  string                     `json:"content,omitempty"`
	ReasoningContent         string                     `json:"reasoning_content,omitempty"`
	AssistantGenMultiContent []schema.MessageOutputPart `json:"assistant_gen_multi_content,omitempty"`
}

// StreamMetaInfo 流开始时的元信息
type StreamMetaInfo struct {
	TreeID    string `json:"tree_id"`
	SessionID string `json:"session"`
	IsNew     bool   `json:"is_new"`
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
