package service

import (
	"context"
	"fmt"

	"devops-backend/internal/api"
	"devops-backend/internal/biz"
)

// chatService 聊天服务实现
type chatService struct {
	chatUsecase    *biz.ChatUsecase
	sessionUsecase *biz.SessionUsecase
}

// NewChatService creates a ChatService.
func NewChatService(chat *biz.ChatUsecase, session *biz.SessionUsecase) api.ChatService {
	return &chatService{
		chatUsecase:    chat,
		sessionUsecase: session,
	}
}

// Chat 执行聊天，进行 DTO 转换
func (s *chatService) Chat(ctx context.Context, req *api.ChatRequest) (*api.ChatResponse, error) {
	bizReq := &biz.ChatRequest{
		Message:  req.Message,
		Model:    req.Model,
		Session:  req.Session,
		Thinking: req.Thinking,
	}

	treeID, sessionID, _, err := s.sessionUsecase.ResolveSession(bizReq.Session)
	if err != nil {
		return nil, fmt.Errorf("resolve session: %w", err)
	}

	userMsg := biz.BuildUserMessage(bizReq)
	if _, err := s.sessionUsecase.AppendMessage(sessionID, userMsg, ""); err != nil {
		return nil, fmt.Errorf("append user message: %w", err)
	}

	messages, err := s.sessionUsecase.GetHistory(sessionID)
	if err != nil {
		return nil, fmt.Errorf("get session history: %w", err)
	}

	result, modelName, err := s.chatUsecase.Chat(ctx, messages, bizReq.Model, bizReq.Thinking)
	if err != nil {
		return nil, err
	}

	if _, err := s.sessionUsecase.AppendMessage(sessionID, result, modelName); err != nil {
		return nil, fmt.Errorf("append assistant message: %w", err)
	}

	return &api.ChatResponse{
		Message:   *result,
		Model:     modelName,
		SessionID: sessionID,
		TreeID:    treeID,
	}, nil
}

// ChatStream 执行流式聊天
func (s *chatService) ChatStream(
	ctx context.Context,
	req *api.ChatRequest,
	onStart api.StreamStartCallback,
	onChunk api.StreamChunkCallback,
) error {
	bizReq := &biz.ChatRequest{
		Message:  req.Message,
		Model:    req.Model,
		Session:  req.Session,
		Thinking: req.Thinking,
	}

	treeID, sessionID, isNew, err := s.sessionUsecase.ResolveSession(bizReq.Session)
	if err != nil {
		return fmt.Errorf("resolve session: %w", err)
	}

	if err := onStart(api.StreamMetaInfo{
		TreeID:    treeID,
		SessionID: sessionID,
		IsNew:     isNew,
	}); err != nil {
		return err
	}

	userMsg := biz.BuildUserMessage(bizReq)
	if _, err := s.sessionUsecase.AppendMessage(sessionID, userMsg, ""); err != nil {
		return fmt.Errorf("append user message: %w", err)
	}

	messages, err := s.sessionUsecase.GetHistory(sessionID)
	if err != nil {
		return fmt.Errorf("get session history: %w", err)
	}

	bizChunkAdapter := func(chunk biz.StreamChunk) error {
		return onChunk(api.StreamChunk{
			Content:                  chunk.Content,
			ReasoningContent:         chunk.ReasoningContent,
			AssistantGenMultiContent: chunk.AssistantGenMultiContent,
		})
	}

	assistantMsg, modelName, err := s.chatUsecase.ChatStream(ctx, messages, bizReq.Model, bizReq.Thinking, bizChunkAdapter)
	if err != nil {
		return err
	}

	if _, err := s.sessionUsecase.AppendMessage(sessionID, assistantMsg, modelName); err != nil {
		return fmt.Errorf("append assistant message: %w", err)
	}

	return nil
}

// ListSessions 列出所有会话树
func (s *chatService) ListSessions(ctx context.Context) ([]api.SessionInfo, error) {
	trees, err := s.sessionUsecase.ListSessions()
	if err != nil {
		return nil, err
	}

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
	session, err := s.sessionUsecase.GetSession(sessionID)
	if err != nil {
		return nil, err
	}

	messages := make([]*api.ChatResponse, len(session))
	for i, msg := range session {
		messages[i] = &api.ChatResponse{
			Message: msg.Message,
			Model:   msg.Model,
		}
	}

	return &api.GetSessionResponse{Messages: messages}, nil
}
