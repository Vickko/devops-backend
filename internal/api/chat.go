package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/cloudwego/eino/schema"
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

// chat 流式聊天接口 (SSE)
func (h *ChatHandler) chat(w http.ResponseWriter, r *http.Request) {
	var req ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body: " + err.Error()})
		return
	}

	// 设置 SSE 响应头
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // 禁用 nginx 缓冲

	// 获取 flusher
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "streaming not supported"})
		return
	}

	// 调用流式服务（双回调）
	err := h.chatService.ChatStream(r.Context(), &req,
		// onStart: 流开始前回调，处理 session 信息
		func(info StreamMetaInfo) error {
			// 总是发送 info 事件，包含 tree_id 和 session
			jsonData, _ := json.Marshal(map[string]interface{}{
				"tree_id": info.TreeID,
				"session": info.SessionID,
				"is_new":  info.IsNew,
			})
			fmt.Fprintf(w, "event: info\ndata: %s\n\n", jsonData)
			flusher.Flush()
			return nil
		},
		// onChunk: 流数据回调
		func(chunk StreamChunk) error {
			return h.sendStreamChunk(w, flusher, chunk)
		},
	)

	if err != nil {
		// session 不存在的错误
		if strings.Contains(err.Error(), "session not found") {
			fmt.Fprintf(w, "event: error\ndata: %s\n\n", `{"error":"invalid_session","message":"Session not found"}`)
			flusher.Flush()
			return
		}
		// 其他错误
		fmt.Fprintf(w, "event: error\ndata: %s\n\n", err.Error())
		flusher.Flush()
		return
	}

	// 发送完成事件
	fmt.Fprintf(w, "event: done\ndata: [DONE]\n\n")
	flusher.Flush()
}

// sendStreamChunk 发送流式数据块
func (h *ChatHandler) sendStreamChunk(w http.ResponseWriter, flusher http.Flusher, chunk StreamChunk) error {
	// 发送思考内容（使用 reasoning 事件）
	if chunk.ReasoningContent != "" {
		jsonData, _ := json.Marshal(chunk.ReasoningContent)
		fmt.Fprintf(w, "event: reasoning\ndata: %s\n\n", jsonData)
		flusher.Flush()
	}

	// 发送最终内容（使用 content 事件）
	if chunk.Content != "" {
		jsonData, _ := json.Marshal(chunk.Content)
		fmt.Fprintf(w, "event: content\ndata: %s\n\n", jsonData)
		flusher.Flush()
	}

	// 发送多模态内容（根据类型分别发送）
	if len(chunk.AssistantGenMultiContent) > 0 {
		for _, part := range chunk.AssistantGenMultiContent {
			var eventName string
			var eventData interface{}

			switch part.Type {
			case schema.ChatMessagePartTypeText:
				// 文本内容已经通过 content 事件发送，跳过
				continue
			case schema.ChatMessagePartTypeImageURL:
				eventName = "image"
				eventData = part.Image
			case schema.ChatMessagePartTypeAudioURL:
				eventName = "audio"
				eventData = part.Audio
			case schema.ChatMessagePartTypeVideoURL:
				eventName = "video"
				eventData = part.Video
			default:
				// 未知类型，使用通用的 multimodal 事件作为后备
				eventName = "multimodal"
				eventData = part
			}

			jsonData, _ := json.Marshal(eventData)
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", eventName, jsonData)
			flusher.Flush()
		}
	}

	return nil
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
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

// getSession 获取会话详情
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
