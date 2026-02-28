package api

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"devops-backend/internal/infra/data/provider"

	"github.com/cloudwego/eino/schema"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
)

const (
	maxInputBinaryParts = 4
	maxInputBinaryBytes = 5 * 1024 * 1024
	maxInputTotalBytes  = 10 * 1024 * 1024
)

const (
	chatErrCodeInvalidRequestBody    = "invalid_request_body"
	chatErrCodeInvalidContentFormat  = "invalid_content_format"
	chatErrCodeUnsupportedPartType   = "unsupported_content_part_type"
	chatErrCodeBinaryDataEmpty       = "binary_data_empty"
	chatErrCodeBinaryMIMERequired    = "binary_mime_required"
	chatErrCodeBinaryMIMEUnsupported = "binary_mime_unsupported"
	chatErrCodeBinaryDecodeFailed    = "binary_decode_failed"
	chatErrCodeBinaryPartTooLarge    = "binary_part_too_large"
	chatErrCodeBinaryTotalTooLarge   = "binary_total_too_large"
	chatErrCodeBinaryPartTooMany     = "binary_part_too_many"
	chatErrCodeModelImageUnsupported = "model_image_unsupported"
)

var allowedInputImageMIMETypes = map[string]struct{}{
	"image/png":  {},
	"image/jpeg": {},
	"image/webp": {},
	"image/gif":  {},
}

type chatInputError struct {
	code string
	msg  string
	err  error
}

func (e *chatInputError) Error() string {
	if e.msg != "" {
		return e.msg
	}
	if e.err != nil {
		return e.err.Error()
	}
	return e.code
}

func (e *chatInputError) Unwrap() error {
	return e.err
}

func newChatInputError(code, msg string) error {
	return &chatInputError{
		code: code,
		msg:  msg,
	}
}

func wrapChatInputError(code, msg string, err error) error {
	return &chatInputError{
		code: code,
		msg:  msg,
		err:  err,
	}
}

func chatInputErrorCode(err error) (string, bool) {
	if err == nil {
		return "", false
	}
	var inputErr *chatInputError
	if errors.As(err, &inputErr) && inputErr.code != "" {
		return inputErr.code, true
	}
	return "", false
}

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
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"code":  chatErrCodeInvalidRequestBody,
			"error": "invalid request body: " + err.Error(),
		})
		return
	}

	req, err := buildChatRequestFromRunInput(&runInput)
	if err != nil {
		resp := map[string]string{"error": err.Error()}
		if code, ok := chatInputErrorCode(err); ok {
			resp["code"] = code
		}
		writeJSON(w, http.StatusBadRequest, resp)
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
	if err := validateModelInputCapabilities(msg, model); err != nil {
		return nil, err
	}
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
	content, parts, err := parseRunAgentContent(msg.Content)
	if err != nil {
		return nil, err
	}
	role := parseRunAgentRole(msg.Role)
	message := &schema.Message{
		Role:       role,
		Content:    content,
		Name:       msg.Name,
		ToolCallID: msg.ToolCallID,
		ToolCalls:  msg.ToolCalls,
	}
	if len(parts) > 0 {
		if role != schema.User || !hasNonTextInputPart(parts) {
			// 纯文本分片保留到 Content，避免同时携带 Content + MultiContent。
			return message, nil
		}
		// 对用户图文混合输入，保留 Content 供会话标题/摘要使用；
		// 在真正调用模型前会在 biz.prepareMessagesForModel 再清理为多模态输入格式。
		message.UserInputMultiContent = parts
	}
	return message, nil
}

func hasNonTextInputPart(parts []schema.MessageInputPart) bool {
	for _, part := range parts {
		if part.Type != schema.ChatMessagePartTypeText {
			return true
		}
	}
	return false
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

func parseRunAgentContent(raw json.RawMessage) (string, []schema.MessageInputPart, error) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return "", nil, nil
	}

	// AG-UI Message.content 仅支持分片模式（text + binary(image)）
	var parts []RunAgentInputContentPart
	if err := json.Unmarshal(raw, &parts); err == nil {
		var sb strings.Builder
		inputParts := make([]schema.MessageInputPart, 0, len(parts))
		binaryParts := 0
		totalBinaryBytes := 0

		for _, part := range parts {
			partType := strings.ToLower(strings.TrimSpace(part.Type))
			switch partType {
			case "text":
				sb.WriteString(part.Text)
				inputParts = append(inputParts, schema.MessageInputPart{
					Type: schema.ChatMessagePartTypeText,
					Text: part.Text,
				})
			case "binary":
				binaryParts++
				if binaryParts > maxInputBinaryParts {
					return "", nil, newChatInputError(
						chatErrCodeBinaryPartTooMany,
						fmt.Sprintf("too many binary parts: max %d", maxInputBinaryParts),
					)
				}

				base64Data, mimeType, err := normalizeBinaryPart(part)
				if err != nil {
					return "", nil, err
				}
				decodedSize, tooLarge, err := decodeBase64Size(base64Data, maxInputBinaryBytes)
				if err != nil {
					return "", nil, wrapChatInputError(chatErrCodeBinaryDecodeFailed, "invalid binary data", err)
				}
				if tooLarge {
					return "", nil, newChatInputError(
						chatErrCodeBinaryPartTooLarge,
						fmt.Sprintf("binary part too large: max %d bytes", maxInputBinaryBytes),
					)
				}
				totalBinaryBytes += decodedSize
				if totalBinaryBytes > maxInputTotalBytes {
					return "", nil, newChatInputError(
						chatErrCodeBinaryTotalTooLarge,
						fmt.Sprintf("total binary data too large: max %d bytes", maxInputTotalBytes),
					)
				}

				base64Value := base64Data
				inputParts = append(inputParts, schema.MessageInputPart{
					Type: schema.ChatMessagePartTypeImageURL,
					Image: &schema.MessageInputImage{
						MessagePartCommon: schema.MessagePartCommon{
							Base64Data: &base64Value,
							MIMEType:   mimeType,
						},
					},
				})
			default:
				return "", nil, newChatInputError(
					chatErrCodeUnsupportedPartType,
					fmt.Sprintf("unsupported content part type: %s", part.Type),
				)
			}
		}
		return sb.String(), inputParts, nil
	}

	return "", nil, newChatInputError(chatErrCodeInvalidContentFormat, "unsupported message content format")
}

func normalizeBinaryPart(part RunAgentInputContentPart) (base64Data string, mimeType string, err error) {
	raw := strings.TrimSpace(part.Data)
	if raw == "" {
		return "", "", newChatInputError(chatErrCodeBinaryDataEmpty, "binary content data is empty")
	}

	mimeType = strings.ToLower(strings.TrimSpace(part.MimeType))
	if strings.HasPrefix(raw, "data:") {
		prefixEnd := strings.Index(raw, ",")
		if prefixEnd <= 0 {
			return "", "", newChatInputError(chatErrCodeInvalidContentFormat, "invalid data url")
		}
		prefix := strings.ToLower(raw[:prefixEnd])
		if !strings.Contains(prefix, ";base64") {
			return "", "", newChatInputError(chatErrCodeInvalidContentFormat, "data url must be base64 encoded")
		}
		if mimeType == "" {
			mime := strings.TrimPrefix(prefix, "data:")
			mimeType = strings.TrimSuffix(mime, ";base64")
		}
		raw = raw[prefixEnd+1:]
	}

	if mimeType == "" {
		return "", "", newChatInputError(chatErrCodeBinaryMIMERequired, "binary content mimeType is required")
	}
	if _, ok := allowedInputImageMIMETypes[mimeType]; !ok {
		return "", "", newChatInputError(
			chatErrCodeBinaryMIMEUnsupported,
			fmt.Sprintf("unsupported image mimeType: %s", mimeType),
		)
	}

	return raw, mimeType, nil
}

func validateModelInputCapabilities(msg *schema.Message, modelName string) error {
	if msg == nil {
		return nil
	}
	if strings.TrimSpace(modelName) == "" {
		return nil
	}
	if !hasImageInputPart(msg.UserInputMultiContent) {
		return nil
	}
	if provider.GetModelCapabilityRegistry().SupportsModality(modelName, provider.ModalityImage) {
		return nil
	}
	return newChatInputError(
		chatErrCodeModelImageUnsupported,
		fmt.Sprintf("model %s does not support image input", modelName),
	)
}

func hasImageInputPart(parts []schema.MessageInputPart) bool {
	for _, part := range parts {
		if part.Type == schema.ChatMessagePartTypeImageURL {
			return true
		}
	}
	return false
}

func decodeBase64Size(data string, limit int) (decodedSize int, tooLarge bool, err error) {
	encodings := []*base64.Encoding{
		base64.StdEncoding,
		base64.RawStdEncoding,
		base64.URLEncoding,
		base64.RawURLEncoding,
	}
	var lastErr error
	for _, enc := range encodings {
		size, exceeded, decodeErr := decodeBase64SizeWithEncoding(data, enc, limit)
		if decodeErr == nil {
			return size, exceeded, nil
		}
		lastErr = decodeErr
	}
	return 0, false, lastErr
}

func decodeBase64SizeWithEncoding(data string, enc *base64.Encoding, limit int) (decodedSize int, tooLarge bool, err error) {
	decoder := base64.NewDecoder(enc, strings.NewReader(data))
	buf := make([]byte, 32*1024)
	total := 0
	for {
		n, readErr := decoder.Read(buf)
		if n > 0 {
			total += n
			if total > limit {
				return total, true, nil
			}
		}
		if readErr == nil {
			continue
		}
		if errors.Is(readErr, io.EOF) {
			return total, false, nil
		}
		return 0, false, readErr
	}
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
