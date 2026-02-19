package biz

import (
	"context"
	"fmt"
	"io"
	"reflect"
	"strings"

	"devops-backend/internal/conf"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

// ChatUsecase handles AI chat execution (agent creation, inference, streaming).
type ChatUsecase struct {
	provider     ChatModelProvider
	defaultModel string
}

// NewChatUsecase creates a ChatUsecase.
func NewChatUsecase(provider ChatModelProvider, cfg conf.Eino) *ChatUsecase {
	return &ChatUsecase{
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
	ThreadID string `json:"thread_id,omitempty"`
	Thinking *bool  `json:"thinking,omitempty"`
}

// ChatResponse 聊天响应
type ChatResponse struct {
	schema.Message
	Model string `json:"model,omitempty"`
}

// BuildUserMessage constructs a schema.Message from the request.
func BuildUserMessage(req *ChatRequest) *schema.Message {
	msg := &schema.Message{
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
	return msg
}

// resolveModel returns the requested model or falls back to the default.
func (uc *ChatUsecase) resolveModel(reqModel string) string {
	if reqModel == "" {
		return uc.defaultModel
	}
	return reqModel
}

// Chat executes a non-streaming chat. It returns the assistant response and the actual model name.
func (uc *ChatUsecase) Chat(
	ctx context.Context,
	messages []*schema.Message,
	reqModel string,
	thinking *bool,
) (*schema.Message, string, error) {
	modelName := uc.resolveModel(reqModel)
	preparedMessages := prepareMessagesForModel(messages)

	agent, err := uc.createAgent(ctx, modelName)
	if err != nil {
		return nil, "", wrapError("create agent", err)
	}

	thinkingOpts := WithParams(&RequestParams{
		Thinking: thinking,
	})
	iter := agent.Run(ctx, &adk.AgentInput{
		Messages:        preparedMessages,
		EnableStreaming: false,
	}, adk.WithChatModelOptions([]model.Option{thinkingOpts}))

	var result *schema.Message
	for {
		event, ok := iter.Next()
		if !ok {
			break
		}
		if event.Err != nil {
			return nil, "", wrapError("agent run", event.Err)
		}
		if event.Output != nil && event.Output.MessageOutput != nil {
			msg, err := event.Output.MessageOutput.GetMessage()
			if err != nil {
				return nil, "", wrapError("get message", err)
			}
			if msg != nil {
				result = msg
			}
		}
	}

	if result == nil {
		return nil, "", wrapError("agent run", fmt.Errorf("no response from agent"))
	}

	return result, modelName, nil
}

// StreamChunk 流式响应块，区分思考内容和最终内容
type StreamChunk struct {
	Content                  string                     `json:"content,omitempty"`
	ReasoningContent         string                     `json:"reasoning_content,omitempty"`
	AssistantGenMultiContent []schema.MessageOutputPart `json:"assistant_gen_multi_content,omitempty"`
	ToolCalls                []schema.ToolCall          `json:"tool_calls,omitempty"`
}

// StreamChunkCallback 流数据回调
type StreamChunkCallback func(chunk StreamChunk) error

// ChatStream executes a streaming chat. It returns the complete assistant response and the actual model name.
func (uc *ChatUsecase) ChatStream(
	ctx context.Context,
	messages []*schema.Message,
	reqModel string,
	thinking *bool,
	onChunk StreamChunkCallback,
) (*schema.Message, string, error) {
	modelName := uc.resolveModel(reqModel)
	preparedMessages := prepareMessagesForModel(messages)

	agent, err := uc.createAgent(ctx, modelName)
	if err != nil {
		return nil, "", wrapError("create agent", err)
	}

	thinkingOpts := WithParams(&RequestParams{
		Thinking: thinking,
	})
	iter := agent.Run(ctx, &adk.AgentInput{
		Messages:        preparedMessages,
		EnableStreaming: true,
	}, adk.WithChatModelOptions([]model.Option{thinkingOpts}))

	// 收集完整回复用于保存会话
	var fullContent strings.Builder
	var fullReasoning strings.Builder
	var multiContent []schema.MessageOutputPart
	var toolCalls []schema.ToolCall

	for {
		event, ok := iter.Next()
		if !ok {
			break
		}
		if event.Err != nil {
			return nil, "", wrapError("agent run", event.Err)
		}
		if event.Output == nil || event.Output.MessageOutput == nil {
			continue
		}

		mv := event.Output.MessageOutput
		if mv.IsStreaming {
			if err := consumeStream(mv.MessageStream, &fullContent, &fullReasoning, &multiContent, &toolCalls, onChunk); err != nil {
				return nil, "", err
			}
		} else if mv.Message != nil {
			streamChunk := StreamChunk{}

			contentDelta := computeStreamSnapshotDelta(fullContent.String(), mv.Message.Content)
			if contentDelta != "" {
				fullContent.WriteString(contentDelta)
				streamChunk.Content = contentDelta
			}

			reasoningDelta := computeStreamSnapshotDelta(fullReasoning.String(), mv.Message.ReasoningContent)
			if reasoningDelta != "" {
				fullReasoning.WriteString(reasoningDelta)
				streamChunk.ReasoningContent = reasoningDelta
			}

			multiContentDelta := computeMultiContentSnapshotDelta(multiContent, mv.Message.AssistantGenMultiContent)
			if len(multiContentDelta) > 0 {
				multiContent = append(multiContent, multiContentDelta...)
				streamChunk.AssistantGenMultiContent = multiContentDelta
			}

			var toolCallsChanged bool
			toolCalls, toolCallsChanged = mergeToolCallsWithChange(toolCalls, mv.Message.ToolCalls)
			if toolCallsChanged {
				streamChunk.ToolCalls = mv.Message.ToolCalls
			}

			if streamChunk.Content != "" || streamChunk.ReasoningContent != "" || len(streamChunk.AssistantGenMultiContent) > 0 || len(streamChunk.ToolCalls) > 0 {
				if cbErr := onChunk(streamChunk); cbErr != nil {
					return nil, "", cbErr
				}
			}
		}
	}

	assistantMsg := &schema.Message{
		Role:                     schema.Assistant,
		Content:                  fullContent.String(),
		ReasoningContent:         fullReasoning.String(),
		AssistantGenMultiContent: multiContent,
		ToolCalls:                toolCalls,
	}
	return assistantMsg, modelName, nil
}

func prepareMessagesForModel(messages []*schema.Message) []*schema.Message {
	prepared := make([]*schema.Message, len(messages))
	for i, msg := range messages {
		if msg == nil {
			continue
		}

		cloned := *msg
		prepared[i] = &cloned

		if len(cloned.UserInputMultiContent) == 0 {
			continue
		}

		if cloned.Role != schema.User {
			if cloned.Content == "" {
				cloned.Content = concatInputTextParts(cloned.UserInputMultiContent)
			}
			cloned.UserInputMultiContent = nil
			continue
		}

		if hasNonTextInputParts(cloned.UserInputMultiContent) {
			cloned.Content = ""
			continue
		}

		if cloned.Content == "" {
			cloned.Content = concatInputTextParts(cloned.UserInputMultiContent)
		}
		cloned.UserInputMultiContent = nil
	}
	return prepared
}

func hasNonTextInputParts(parts []schema.MessageInputPart) bool {
	for _, part := range parts {
		if part.Type != schema.ChatMessagePartTypeText {
			return true
		}
	}
	return false
}

func concatInputTextParts(parts []schema.MessageInputPart) string {
	var sb strings.Builder
	for _, part := range parts {
		if part.Type == schema.ChatMessagePartTypeText {
			sb.WriteString(part.Text)
		}
	}
	return sb.String()
}

func computeStreamSnapshotDelta(current, snapshot string) string {
	if snapshot == "" {
		return ""
	}
	if current == "" {
		return snapshot
	}
	if strings.HasPrefix(snapshot, current) {
		return strings.TrimPrefix(snapshot, current)
	}
	// 非前缀快照无法安全增量化，直接忽略避免重复输出。
	return ""
}

func computeMultiContentSnapshotDelta(current, snapshot []schema.MessageOutputPart) []schema.MessageOutputPart {
	if len(snapshot) == 0 {
		return nil
	}
	if len(current) == 0 {
		return append([]schema.MessageOutputPart(nil), snapshot...)
	}
	if len(snapshot) < len(current) {
		return nil
	}
	if !reflect.DeepEqual(snapshot[:len(current)], current) {
		return nil
	}
	return append([]schema.MessageOutputPart(nil), snapshot[len(current):]...)
}

func mergeToolCallsWithChange(existing, incoming []schema.ToolCall) ([]schema.ToolCall, bool) {
	if len(incoming) == 0 {
		return existing, false
	}
	merged := mergeToolCalls(existing, incoming)
	return merged, !reflect.DeepEqual(existing, merged)
}

// consumeStream reads all frames from a MessageStream, accumulates content, and calls onChunk.
// The stream is always closed when this function returns.
func consumeStream(
	stream *schema.StreamReader[*schema.Message],
	fullContent, fullReasoning *strings.Builder,
	multiContent *[]schema.MessageOutputPart,
	toolCalls *[]schema.ToolCall,
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
			ToolCalls:                chunk.ToolCalls,
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
		if len(chunk.ToolCalls) > 0 {
			*toolCalls = mergeToolCalls(*toolCalls, chunk.ToolCalls)
		}

		if sc.Content != "" || sc.ReasoningContent != "" || len(sc.AssistantGenMultiContent) > 0 || len(sc.ToolCalls) > 0 {
			if cbErr := onChunk(sc); cbErr != nil {
				return cbErr
			}
		}
	}
}

// mergeToolCalls merges streamed tool call chunks into stable tool call entries.
func mergeToolCalls(existing, incoming []schema.ToolCall) []schema.ToolCall {
	if len(incoming) == 0 {
		return existing
	}

	result := append([]schema.ToolCall(nil), existing...)
	idIndex := make(map[string]int, len(result))
	indexIndex := make(map[int]int, len(result))
	for i, call := range result {
		if call.ID != "" {
			idIndex[call.ID] = i
		}
		if call.Index != nil {
			indexIndex[*call.Index] = i
		}
	}

	for _, chunk := range incoming {
		target := -1
		if chunk.ID != "" {
			if idx, ok := idIndex[chunk.ID]; ok {
				target = idx
			}
		}
		if target < 0 && chunk.Index != nil {
			if idx, ok := indexIndex[*chunk.Index]; ok {
				target = idx
			}
		}

		if target < 0 {
			result = append(result, chunk)
			newIdx := len(result) - 1
			if chunk.ID != "" {
				idIndex[chunk.ID] = newIdx
			}
			if chunk.Index != nil {
				indexIndex[*chunk.Index] = newIdx
			}
			continue
		}

		current := result[target]
		if chunk.ID != "" {
			current.ID = chunk.ID
		}
		if chunk.Type != "" {
			current.Type = chunk.Type
		}
		if chunk.Index != nil {
			current.Index = chunk.Index
		}
		if chunk.Function.Name != "" {
			current.Function.Name = chunk.Function.Name
		}
		if chunk.Function.Arguments != "" {
			if current.Function.Arguments == "" || strings.HasPrefix(chunk.Function.Arguments, current.Function.Arguments) {
				current.Function.Arguments = chunk.Function.Arguments
			} else {
				current.Function.Arguments += chunk.Function.Arguments
			}
		}
		if len(chunk.Extra) > 0 {
			if current.Extra == nil {
				current.Extra = map[string]any{}
			}
			for k, v := range chunk.Extra {
				current.Extra[k] = v
			}
		}

		result[target] = current
	}

	return result
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

// wrapError wraps an error with an operation context.
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
