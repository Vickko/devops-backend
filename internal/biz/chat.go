package biz

import (
	"context"
	"fmt"
	"io"
	"strings"

	"devops-backend/internal/conf"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

// ChatUsecase 聊天业务逻辑
type ChatUsecase struct {
	sessionRepo  SessionRepo
	provider     ChatModelProvider
	defaultModel string
}

// NewChatUsecase 创建 ChatUsecase
func NewChatUsecase(sessionRepo SessionRepo, provider ChatModelProvider, cfg conf.Eino) *ChatUsecase {
	return &ChatUsecase{
		sessionRepo:  sessionRepo,
		provider:     provider,
		defaultModel: cfg.DefaultModel,
	}
}

// createAgent builds a ChatModelAgent for the given model name.
func (uc *ChatUsecase) createAgent(ctx context.Context, modelName string) (*adk.ChatModelAgent, error) {
	chatModel, err := uc.provider.CreateChatModel(ctx, modelName)
	if err != nil {
		return nil, err
	}
	return adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
		Name:        "chat_assistant",
		Description: "友好的AI聊天助手",
		Instruction: "你是一个友好的AI助手，请用简洁明了的方式回答用户的问题。",
		Model:       chatModel,
	})
}

// ChatRequest 聊天请求
type ChatRequest struct {
	schema.Message
	Model    string `json:"model,omitempty"`
	Session  string `json:"session"`
	Thinking *bool  `json:"thinking,omitempty"`
}

// ChatResponse 聊天响应
type ChatResponse struct {
	schema.Message
	Model string `json:"model,omitempty"`
}

// Chat 执行聊天
func (uc *ChatUsecase) Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
	// 构建用户消息
	userMsg := &schema.Message{
		Role:                     parseRole(string(req.Role)),
		Content:                  req.Content,
		Name:                     req.Name,
		UserInputMultiContent:    req.UserInputMultiContent,
		AssistantGenMultiContent: req.AssistantGenMultiContent,
		ToolCalls:                req.ToolCalls,
		ToolCallID:               req.ToolCallID,
		ToolName:                 req.ToolName,
		ReasoningContent:         req.ReasoningContent,
		Extra:                    req.Extra,
	}

	// 追加用户消息到会话
	if _, err := uc.sessionRepo.AppendMessage(req.Session, userMsg, ""); err != nil {
		return nil, wrapError("append user message", err)
	}

	// 获取会话消息列表（包含刚追加的用户消息）
	session := uc.sessionRepo.GetSessionMessages(req.Session)
	if session == nil {
		return nil, wrapError("get session", ErrSessionNotFound)
	}

	// 构建消息列表
	messages := extractMessages(session)

	// 确定使用的模型
	modelName := req.Model
	if modelName == "" {
		modelName = uc.defaultModel
	}

	agent, err := uc.createAgent(ctx, modelName)
	if err != nil {
		return nil, wrapError("create agent", err)
	}

	// 运行 agent（非流式）
	thinkingOpts := WithParams(&RequestParams{
		Thinking: req.Thinking,
	})
	iter := agent.Run(ctx, &adk.AgentInput{
		Messages:       messages,
		EnableStreaming: false,
	}, adk.WithChatModelOptions([]model.Option{thinkingOpts}))

	// 收集结果
	var result *schema.Message
	for {
		event, ok := iter.Next()
		if !ok {
			break
		}
		if event.Err != nil {
			return nil, wrapError("agent run", event.Err)
		}
		if event.Output != nil && event.Output.MessageOutput != nil {
			msg, err := event.Output.MessageOutput.GetMessage()
			if err != nil {
				return nil, wrapError("get message", err)
			}
			if msg != nil {
				result = msg
			}
		}
	}

	if result == nil {
		return nil, wrapError("agent run", fmt.Errorf("no response from agent"))
	}

	// 追加助手回复到会话历史
	if _, err := uc.sessionRepo.AppendMessage(req.Session, result, modelName); err != nil {
		return nil, wrapError("append assistant message", err)
	}

	return &ChatResponse{Message: *result, Model: modelName}, nil
}

// StreamChunk 流式响应块，区分思考内容和最终内容
type StreamChunk struct {
	Content                  string                     `json:"content,omitempty"`
	ReasoningContent         string                     `json:"reasoning_content,omitempty"`
	AssistantGenMultiContent []schema.MessageOutputPart `json:"assistant_gen_multi_content,omitempty"`
}

// StreamMetaInfo 流开始时的元信息
type StreamMetaInfo struct {
	TreeID    string
	SessionID string
	IsNew     bool
}

// StreamStartCallback 流开始时的回调，传递元信息
type StreamStartCallback func(info StreamMetaInfo) error

// StreamChunkCallback 流数据回调
type StreamChunkCallback func(chunk StreamChunk) error

// ChatStream 执行流式聊天
// onStart: 流开始前回调，传递 session 信息
// onChunk: 流数据回调
func (uc *ChatUsecase) ChatStream(
	ctx context.Context,
	req *ChatRequest,
	onStart StreamStartCallback,
	onChunk StreamChunkCallback,
) error {
	// 1. session 验证/创建
	var treeID, sessionID string
	isNew := false

	if req.Session == "" {
		treeID, sessionID = uc.sessionRepo.NewConversation()
		isNew = true
	} else {
		sessionID = req.Session
		var err error
		treeID, err = uc.sessionRepo.GetTreeID(sessionID)
		if err != nil {
			return fmt.Errorf("session not found: %s", sessionID)
		}
	}

	// 2. 通知调用者 session 信息（在流开始之前）
	if err := onStart(StreamMetaInfo{TreeID: treeID, SessionID: sessionID, IsNew: isNew}); err != nil {
		return err
	}

	// 3. 构建用户消息
	userMsg := &schema.Message{
		Role:                     parseRole(string(req.Role)),
		Content:                  req.Content,
		Name:                     req.Name,
		UserInputMultiContent:    req.UserInputMultiContent,
		AssistantGenMultiContent: req.AssistantGenMultiContent,
		ToolCalls:                req.ToolCalls,
		ToolCallID:               req.ToolCallID,
		ToolName:                 req.ToolName,
		ReasoningContent:         req.ReasoningContent,
		Extra:                    req.Extra,
	}

	// 追加用户消息到会话
	if _, err := uc.sessionRepo.AppendMessage(sessionID, userMsg, ""); err != nil {
		return wrapError("append user message", err)
	}

	// 获取会话消息列表（包含刚追加的用户消息）
	session := uc.sessionRepo.GetSessionMessages(sessionID)
	if session == nil {
		return wrapError("get session", ErrSessionNotFound)
	}

	// 构建消息列表（instruction 已在 agent config 中，无需手动添加系统提示）
	messages := extractMessages(session)

	// 确定使用的模型
	modelName := req.Model
	if modelName == "" {
		modelName = uc.defaultModel
	}

	// 创建 agent
	agent, err := uc.createAgent(ctx, modelName)
	if err != nil {
		return wrapError("create agent", err)
	}

	// 运行 agent（流式）
	thinkingOpts := WithParams(&RequestParams{
		Thinking: req.Thinking,
	})
	iter := agent.Run(ctx, &adk.AgentInput{
		Messages:       messages,
		EnableStreaming: true,
	}, adk.WithChatModelOptions([]model.Option{thinkingOpts}))

	// 收集完整回复用于保存会话
	var fullContent strings.Builder
	var fullReasoning strings.Builder
	var multiContent []schema.MessageOutputPart

	// 遍历事件
	for {
		event, ok := iter.Next()
		if !ok {
			break
		}
		if event.Err != nil {
			return wrapError("agent run", event.Err)
		}
		if event.Output == nil || event.Output.MessageOutput == nil {
			continue
		}

		mv := event.Output.MessageOutput
		if mv.IsStreaming {
			if err := consumeStream(mv.MessageStream, &fullContent, &fullReasoning, &multiContent, onChunk); err != nil {
				return err
			}
		} else if mv.Message != nil {
			// 非流式：直接从 Message 提取
			streamChunk := StreamChunk{
				Content:                  mv.Message.Content,
				ReasoningContent:         mv.Message.ReasoningContent,
				AssistantGenMultiContent: mv.Message.AssistantGenMultiContent,
			}

			if mv.Message.ReasoningContent != "" {
				fullReasoning.WriteString(mv.Message.ReasoningContent)
			}
			if mv.Message.Content != "" {
				fullContent.WriteString(mv.Message.Content)
			}
			if len(mv.Message.AssistantGenMultiContent) > 0 {
				multiContent = append(multiContent, mv.Message.AssistantGenMultiContent...)
			}

			if streamChunk.Content != "" || streamChunk.ReasoningContent != "" || len(streamChunk.AssistantGenMultiContent) > 0 {
				if cbErr := onChunk(streamChunk); cbErr != nil {
					return cbErr
				}
			}
		}
	}

	// 追加助手回复到会话历史
	assistantMsg := &schema.Message{
		Role:                     schema.Assistant,
		Content:                  fullContent.String(),
		ReasoningContent:         fullReasoning.String(),
		AssistantGenMultiContent: multiContent,
	}
	if _, err := uc.sessionRepo.AppendMessage(sessionID, assistantMsg, modelName); err != nil {
		return wrapError("append assistant message", err)
	}

	return nil
}

// consumeStream reads all frames from a MessageStream, accumulates content, and calls onChunk.
// The stream is always closed when this function returns.
func consumeStream(
	stream *schema.StreamReader[*schema.Message],
	fullContent, fullReasoning *strings.Builder,
	multiContent *[]schema.MessageOutputPart,
	onChunk StreamChunkCallback,
) error {
	defer stream.Close()
	for {
		chunk, err := stream.Recv()
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return wrapError("recv stream", err)
		}

		sc := StreamChunk{
			Content:                  chunk.Content,
			ReasoningContent:         chunk.ReasoningContent,
			AssistantGenMultiContent: chunk.AssistantGenMultiContent,
		}

		if chunk.ReasoningContent != "" {
			fullReasoning.WriteString(chunk.ReasoningContent)
		}
		if chunk.Content != "" {
			fullContent.WriteString(chunk.Content)
		}
		if len(chunk.AssistantGenMultiContent) > 0 {
			*multiContent = append(*multiContent, chunk.AssistantGenMultiContent...)
		}

		if sc.Content != "" || sc.ReasoningContent != "" || len(sc.AssistantGenMultiContent) > 0 {
			if cbErr := onChunk(sc); cbErr != nil {
				return cbErr
			}
		}
	}
}

func parseRole(role string) schema.RoleType {
	switch role {
	case "system":
		return schema.System
	case "assistant":
		return schema.Assistant
	case "tool":
		return schema.Tool
	default:
		return schema.User
	}
}

// extractMessages 从 Session 中提取 schema.Message 列表
func extractMessages(session Session) []*schema.Message {
	msgs := make([]*schema.Message, len(session))
	for i, cr := range session {
		msgs[i] = &cr.Message
	}
	return msgs
}

// wrapError 包装错误信息
func wrapError(op string, err error) error {
	return &chatError{op: op, err: err}
}

type chatError struct {
	op  string
	err error
}

func (e *chatError) Error() string {
	return e.op + ": " + e.err.Error()
}

func (e *chatError) Unwrap() error {
	return e.err
}

// ListSessions 列出所有会话树
func (uc *ChatUsecase) ListSessions() ([]SessionTreeInfo, error) {
	return uc.sessionRepo.ListTrees()
}

// GetSession 获取会话消息列表
func (uc *ChatUsecase) GetSession(sessionID string) (Session, error) {
	session := uc.sessionRepo.GetSessionMessages(sessionID)
	if session == nil {
		return nil, ErrSessionNotFound
	}
	return session, nil
}
