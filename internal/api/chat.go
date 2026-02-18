package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/cloudwego/eino/schema"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
)

// ChatHandler 聊天接口处理器
type ChatHandler struct {
	chatService ChatService
}

// NewChatHandler 创建 ChatHandler
func NewChatHandler(chatService ChatService) *ChatHandler {
	return &ChatHandler{
		chatService: chatService,
	}
}

// RegisterRoutes 注册路由到 mux.Router
func (h *ChatHandler) RegisterRoutes(r *mux.Router) {
	r.HandleFunc("/chat", h.chat).Methods(http.MethodPost)
	r.HandleFunc("/sessions", h.listSessions).Methods(http.MethodGet)
	r.HandleFunc("/sessions/{id}", h.getSession).Methods(http.MethodGet)
}

// chat 流式聊天接口（AG-UI SSE）
func (h *ChatHandler) chat(w http.ResponseWriter, r *http.Request) {
	var runInput RunAgentInput
	if err := json.NewDecoder(r.Body).Decode(&runInput); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body: " + err.Error()})
		return
	}

	req, err := buildChatRequestFromRunInput(&runInput)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	// 设置 SSE 响应头
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // 禁用 nginx 缓冲

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "streaming not supported"})
		return
	}

	encoder := newAGUIStreamEncoder(w, flusher, req.ThreadID, req.RunID)

	err = h.chatService.ChatStream(r.Context(), req,
		func(info StreamMetaInfo) error {
			return encoder.onStart(info)
		},
		func(chunk StreamChunk) error {
			return encoder.onChunk(chunk)
		},
	)
	if err != nil {
		code := "internal_error"
		switch {
		case strings.Contains(err.Error(), "session tree not found"):
			code = "invalid_thread"
		case strings.Contains(err.Error(), "session not found"):
			code = "invalid_session"
		}
		_ = encoder.onError(code, err.Error())
		return
	}

	_ = encoder.onDone()
}

func buildChatRequestFromRunInput(input *RunAgentInput) (*ChatRequest, error) {
	if input == nil {
		return nil, fmt.Errorf("request body is required")
	}
	if len(input.Messages) == 0 {
		return nil, fmt.Errorf("messages is required")
	}

	lastMsg := input.Messages[len(input.Messages)-1]
	msg, err := parseRunAgentMessage(lastMsg)
	if err != nil {
		return nil, err
	}

	model, thinking := parseForwardedProps(input.ForwardedProps)
	runID := strings.TrimSpace(input.RunID)
	if runID == "" {
		runID = "run_" + uuid.NewString()
	}

	return &ChatRequest{
		Message:  *msg,
		Model:    model,
		ThreadID: strings.TrimSpace(input.ThreadID),
		RunID:    runID,
		Thinking: thinking,
	}, nil
}

func parseRunAgentMessage(msg RunAgentInputMessage) (*schema.Message, error) {
	content, err := parseRunAgentContent(msg.Content)
	if err != nil {
		return nil, err
	}

	return &schema.Message{
		Role:       parseRunAgentRole(msg.Role),
		Content:    content,
		Name:       msg.Name,
		ToolCallID: msg.ToolCallID,
		ToolCalls:  msg.ToolCalls,
	}, nil
}

func parseRunAgentRole(role string) schema.RoleType {
	switch strings.ToLower(strings.TrimSpace(role)) {
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

func parseRunAgentContent(raw json.RawMessage) (string, error) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return "", nil
	}

	// AG-UI Message.content 的文本模式（字符串）
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return text, nil
	}

	// AG-UI Message.content 的分片模式（当前仅支持 text）
	var parts []RunAgentInputContentPart
	if err := json.Unmarshal(raw, &parts); err == nil {
		var sb strings.Builder
		for _, part := range parts {
			switch part.Type {
			case "text":
				sb.WriteString(part.Text)
			case "binary":
				return "", fmt.Errorf("binary content is not supported yet")
			default:
				return "", fmt.Errorf("unsupported content part type: %s", part.Type)
			}
		}
		return sb.String(), nil
	}

	return "", fmt.Errorf("unsupported message content format")
}

func parseForwardedProps(props map[string]any) (model string, thinking *bool) {
	if props == nil {
		return "", nil
	}

	if rawModel, ok := props["model"]; ok {
		if modelStr, ok := rawModel.(string); ok {
			model = modelStr
		}
	}

	if rawThinking, ok := props["thinking"]; ok {
		if thinkingVal, ok := rawThinking.(bool); ok {
			thinking = &thinkingVal
		}
	}

	return model, thinking
}

type aguiToolCallState struct {
	toolCallName string
	lastArgs     string
}

type aguiStreamEncoder struct {
	w       http.ResponseWriter
	flusher http.Flusher

	threadID string
	runID    string

	assistantMessageID string
	textStarted        bool
	reasoningStarted   bool
	toolCalls          map[string]*aguiToolCallState
}

func newAGUIStreamEncoder(w http.ResponseWriter, flusher http.Flusher, threadID, runID string) *aguiStreamEncoder {
	return &aguiStreamEncoder{
		w:         w,
		flusher:   flusher,
		threadID:  threadID,
		runID:     runID,
		toolCalls: make(map[string]*aguiToolCallState),
	}
}

func (e *aguiStreamEncoder) onStart(info StreamMetaInfo) error {
	e.threadID = info.ThreadID
	e.runID = info.RunID

	return e.writeEvent(aguiRunStartedEvent{
		Type:     "RUN_STARTED",
		ThreadID: info.ThreadID,
		RunID:    info.RunID,
	})
}

func (e *aguiStreamEncoder) onChunk(chunk StreamChunk) error {
	if len(chunk.ToolCalls) > 0 {
		if err := e.emitToolCalls(chunk.ToolCalls); err != nil {
			return err
		}
	}

	if chunk.ReasoningContent != "" {
		if err := e.ensureTextMessageStarted(); err != nil {
			return err
		}
		if !e.reasoningStarted {
			if err := e.writeEvent(aguiTextReasoningStartEvent{
				Type:      "TEXT_MESSAGE_REASONING_START",
				MessageID: e.assistantMessageID,
			}); err != nil {
				return err
			}
			e.reasoningStarted = true
		}
		if err := e.writeEvent(aguiTextReasoningDeltaEvent{
			Type:      "TEXT_MESSAGE_REASONING_DELTA",
			MessageID: e.assistantMessageID,
			Delta:     chunk.ReasoningContent,
		}); err != nil {
			return err
		}
	}

	if chunk.Content != "" {
		if err := e.ensureTextMessageStarted(); err != nil {
			return err
		}
		if err := e.writeEvent(aguiTextMessageDeltaEvent{
			Type:      "TEXT_MESSAGE_DELTA",
			MessageID: e.assistantMessageID,
			Delta:     chunk.Content,
		}); err != nil {
			return err
		}
	}

	return nil
}

func (e *aguiStreamEncoder) onDone() error {
	if err := e.closeOpenStreams(); err != nil {
		return err
	}

	return e.writeEvent(aguiRunFinishedEvent{
		Type:     "RUN_FINISHED",
		ThreadID: e.threadID,
		RunID:    e.runID,
	})
}

func (e *aguiStreamEncoder) onError(code, message string) error {
	if err := e.closeOpenStreams(); err != nil {
		return err
	}

	return e.writeEvent(aguiRunErrorEvent{
		Type:    "RUN_ERROR",
		Code:    code,
		Message: message,
	})
}

func (e *aguiStreamEncoder) closeOpenStreams() error {
	if e.reasoningStarted {
		if err := e.writeEvent(aguiTextReasoningEndEvent{
			Type:      "TEXT_MESSAGE_REASONING_END",
			MessageID: e.assistantMessageID,
		}); err != nil {
			return err
		}
		e.reasoningStarted = false
	}

	for toolCallID, state := range e.toolCalls {
		if state == nil {
			continue
		}
		if err := e.writeEvent(aguiToolCallEndEvent{
			Type:         "TOOL_CALL_END",
			ToolCallID:   toolCallID,
			ToolCallName: state.toolCallName,
			ParentMsgID:  e.assistantMessageID,
		}); err != nil {
			return err
		}
	}
	e.toolCalls = make(map[string]*aguiToolCallState)

	if e.textStarted {
		if err := e.writeEvent(aguiTextMessageEndEvent{
			Type:      "TEXT_MESSAGE_END",
			MessageID: e.assistantMessageID,
		}); err != nil {
			return err
		}
		e.textStarted = false
	}

	return nil
}

func (e *aguiStreamEncoder) emitToolCalls(calls []schema.ToolCall) error {
	if err := e.ensureTextMessageStarted(); err != nil {
		return err
	}

	for _, call := range calls {
		toolCallID := strings.TrimSpace(call.ID)
		if toolCallID == "" && call.Index != nil {
			toolCallID = fmt.Sprintf("tool_%d", *call.Index)
		}
		if toolCallID == "" {
			toolCallID = "tool_" + uuid.NewString()
		}

		state, exists := e.toolCalls[toolCallID]
		if !exists {
			state = &aguiToolCallState{toolCallName: call.Function.Name}
			e.toolCalls[toolCallID] = state
			if err := e.writeEvent(aguiToolCallStartEvent{
				Type:         "TOOL_CALL_START",
				ToolCallID:   toolCallID,
				ToolCallName: call.Function.Name,
				ParentMsgID:  e.assistantMessageID,
			}); err != nil {
				return err
			}
		}

		state.toolCallName = call.Function.Name
		args := strings.TrimSpace(call.Function.Arguments)
		if args != "" && args != state.lastArgs {
			if err := e.writeEvent(aguiToolCallArgsEvent{
				Type:        "TOOL_CALL_ARGS",
				ToolCallID:  toolCallID,
				Args:        parseToolCallArgs(args),
				ParentMsgID: e.assistantMessageID,
			}); err != nil {
				return err
			}
			state.lastArgs = args
		}
	}

	return nil
}

func parseToolCallArgs(args string) any {
	var parsed any
	if err := json.Unmarshal([]byte(args), &parsed); err == nil {
		return parsed
	}
	return map[string]string{"raw": args}
}

func (e *aguiStreamEncoder) ensureTextMessageStarted() error {
	if e.assistantMessageID == "" {
		e.assistantMessageID = "msg_" + uuid.NewString()
	}
	if e.textStarted {
		return nil
	}

	if err := e.writeEvent(aguiTextMessageStartEvent{
		Type:      "TEXT_MESSAGE_START",
		MessageID: e.assistantMessageID,
		Role:      string(schema.Assistant),
	}); err != nil {
		return err
	}

	e.textStarted = true
	return nil
}

func (e *aguiStreamEncoder) writeEvent(event any) error {
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(e.w, "data: %s\n\n", data); err != nil {
		return err
	}
	e.flusher.Flush()
	return nil
}

type aguiRunStartedEvent struct {
	Type     string `json:"type"`
	ThreadID string `json:"threadId"`
	RunID    string `json:"runId"`
}

type aguiRunFinishedEvent struct {
	Type     string `json:"type"`
	ThreadID string `json:"threadId"`
	RunID    string `json:"runId"`
}

type aguiRunErrorEvent struct {
	Type    string `json:"type"`
	Code    string `json:"code,omitempty"`
	Message string `json:"message"`
}

type aguiTextMessageStartEvent struct {
	Type      string `json:"type"`
	MessageID string `json:"messageId"`
	Role      string `json:"role"`
}

type aguiTextMessageDeltaEvent struct {
	Type      string `json:"type"`
	MessageID string `json:"messageId"`
	Delta     string `json:"delta"`
}

type aguiTextMessageEndEvent struct {
	Type      string `json:"type"`
	MessageID string `json:"messageId"`
}

type aguiTextReasoningStartEvent struct {
	Type      string `json:"type"`
	MessageID string `json:"messageId"`
}

type aguiTextReasoningDeltaEvent struct {
	Type      string `json:"type"`
	MessageID string `json:"messageId"`
	Delta     string `json:"delta"`
}

type aguiTextReasoningEndEvent struct {
	Type      string `json:"type"`
	MessageID string `json:"messageId"`
}

type aguiToolCallStartEvent struct {
	Type         string `json:"type"`
	ToolCallID   string `json:"toolCallId"`
	ToolCallName string `json:"toolCallName,omitempty"`
	ParentMsgID  string `json:"parentMessageId,omitempty"`
}

type aguiToolCallArgsEvent struct {
	Type        string `json:"type"`
	ToolCallID  string `json:"toolCallId"`
	Args        any    `json:"args"`
	ParentMsgID string `json:"parentMessageId,omitempty"`
}

type aguiToolCallEndEvent struct {
	Type         string `json:"type"`
	ToolCallID   string `json:"toolCallId"`
	ToolCallName string `json:"toolCallName,omitempty"`
	ParentMsgID  string `json:"parentMessageId,omitempty"`
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

// listSessions 获取会话列表
func (h *ChatHandler) listSessions(w http.ResponseWriter, r *http.Request) {
	sessions, err := h.chatService.ListSessions(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, ListSessionsResponse{Sessions: sessions})
}

// getSession 获取会话详情（支持 session_id 或 tree_id）
func (h *ChatHandler) getSession(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	sessionID := vars["id"]

	resp, err := h.chatService.GetSession(r.Context(), sessionID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "session not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, resp)
}
