package service

import (
	"context"

	"devops-backend/internal/api"
	"devops-backend/internal/biz"
)

// chatService 聊天服务实现
type chatService struct {
	chatUsecase *biz.ChatUsecase
}

// NewChatService 创建 ChatService
func NewChatService(chatUsecase *biz.ChatUsecase) api.ChatService {
	return &chatService{
		chatUsecase: chatUsecase,
	}
}

// Chat 执行聊天，进行 DTO 转换
func (s *chatService) Chat(ctx context.Context, req *api.ChatRequest) (*api.ChatResponse, error) {
	// api DTO -> biz request
	bizReq := &biz.ChatRequest{
		Message:  req.Message,
		Model:    req.Model,
		Client:   req.Client,
		Session:  req.Session,
		Thinking: req.Thinking,
	}

	// 调用业务层
	bizResp, err := s.chatUsecase.Chat(ctx, bizReq)
	if err != nil {
		return nil, err
	}

	// biz response -> api DTO
	return &api.ChatResponse{
		Message: bizResp.Message,
		Model:   bizResp.Model,
	}, nil
}

// ChatStream 执行流式聊天
func (s *chatService) ChatStream(
	ctx context.Context,
	req *api.ChatRequest,
	onStart api.StreamStartCallback,
	onChunk api.StreamChunkCallback,
) error {
	// api DTO -> biz request
	bizReq := &biz.ChatRequest{
		Message:  req.Message,
		Model:    req.Model,
		Client:   req.Client,
		Session:  req.Session,
		Thinking: req.Thinking,
	}

	// 调用业务层流式方法，转换回调类型
	return s.chatUsecase.ChatStream(ctx, bizReq,
		// onStart 回调适配类型：biz.StreamMetaInfo -> api.StreamMetaInfo
		func(info biz.StreamMetaInfo) error {
			return onStart(api.StreamMetaInfo{
				TreeID:    info.TreeID,
				SessionID: info.SessionID,
				IsNew:     info.IsNew,
			})
		},
		// onChunk 回调转换类型
		func(chunk biz.StreamChunk) error {
			return onChunk(api.StreamChunk{
				Content:                  chunk.Content,
				ReasoningContent:         chunk.ReasoningContent,
				AssistantGenMultiContent: chunk.AssistantGenMultiContent,
			})
		},
	)
}

// ListSessions 列出所有会话树
func (s *chatService) ListSessions(ctx context.Context) ([]api.SessionInfo, error) {
	trees, err := s.chatUsecase.ListSessions()
	if err != nil {
		return nil, err
	}

	// biz -> api DTO 转换
	result := make([]api.SessionInfo, len(trees))
	for i, tree := range trees {
		result[i] = api.SessionInfo{
			ID:                  tree.ID,
			Title:               tree.Title,
			LastActiveSessionID: tree.LastActiveSessionID,
			LastMessage:         tree.LastMessage,
			CreatedAt:           tree.CreatedAt,
			UpdatedAt:           tree.UpdatedAt,
		}
	}
	return result, nil
}

// GetSession 获取会话详情
func (s *chatService) GetSession(ctx context.Context, sessionID string) (*api.GetSessionResponse, error) {
	session, err := s.chatUsecase.GetSession(sessionID)
	if err != nil {
		return nil, err
	}

	// biz.Session ([]*biz.ChatResponse) -> []*api.ChatResponse
	messages := make([]*api.ChatResponse, len(session))
	for i, msg := range session {
		messages[i] = &api.ChatResponse{
			Message: msg.Message,
			Model:   msg.Model,
		}
	}

	return &api.GetSessionResponse{Messages: messages}, nil
}
