package api

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestParseRunAgentContent_SuccessTextAndBinary(t *testing.T) {
	raw := mustMarshalJSON(t, []RunAgentInputContentPart{
		{Type: "text", Text: "hello "},
		{Type: "binary", MimeType: "image/png", Data: "aGVsbG8="},
		{Type: "text", Text: "world"},
	})

	content, parts, err := parseRunAgentContent(raw)
	if err != nil {
		t.Fatalf("parseRunAgentContent returned error: %v", err)
	}
	if content != "hello world" {
		t.Fatalf("unexpected content: %q", content)
	}
	if len(parts) != 3 {
		t.Fatalf("unexpected parts length: %d", len(parts))
	}
	if parts[0].Text != "hello " {
		t.Fatalf("unexpected first part text: %q", parts[0].Text)
	}
	if parts[1].Image == nil || parts[1].Image.MIMEType != "image/png" {
		t.Fatalf("unexpected image mimeType: %#v", parts[1].Image)
	}
	if parts[2].Text != "world" {
		t.Fatalf("unexpected third part text: %q", parts[2].Text)
	}
}

func TestParseRunAgentContent_SuccessBinaryOnly(t *testing.T) {
	raw := mustMarshalJSON(t, []RunAgentInputContentPart{
		{Type: "binary", MimeType: "image/png", Data: "aGVsbG8="},
	})

	content, parts, err := parseRunAgentContent(raw)
	if err != nil {
		t.Fatalf("parseRunAgentContent returned error: %v", err)
	}
	if content != "" {
		t.Fatalf("unexpected content: %q", content)
	}
	if len(parts) != 1 {
		t.Fatalf("unexpected parts length: %d", len(parts))
	}
	if parts[0].Image == nil || parts[0].Image.MIMEType != "image/png" {
		t.Fatalf("unexpected image part: %#v", parts[0].Image)
	}
}

func TestParseRunAgentContent_SuccessDataURL(t *testing.T) {
	raw := mustMarshalJSON(t, []RunAgentInputContentPart{
		{Type: "binary", Data: "data:image/png;base64,aGVsbG8="},
	})

	_, parts, err := parseRunAgentContent(raw)
	if err != nil {
		t.Fatalf("parseRunAgentContent returned error: %v", err)
	}
	if len(parts) != 1 {
		t.Fatalf("unexpected parts length: %d", len(parts))
	}
	if parts[0].Image == nil || parts[0].Image.MIMEType != "image/png" {
		t.Fatalf("unexpected image mimeType: %#v", parts[0].Image)
	}
}

func TestParseRunAgentContent_ErrorCodes(t *testing.T) {
	oversizedData := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte("a"), maxInputBinaryBytes+1))
	totalPartData := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte("a"), 4*1024*1024))

	cases := []struct {
		name string
		raw  json.RawMessage
		want string
	}{
		{
			name: "invalid content format",
			raw:  json.RawMessage(`123`),
			want: chatErrCodeInvalidContentFormat,
		},
		{
			name: "legacy plain text content is rejected",
			raw:  json.RawMessage(`"hello"`),
			want: chatErrCodeInvalidContentFormat,
		},
		{
			name: "unsupported content part type",
			raw: mustMarshalJSON(t, []RunAgentInputContentPart{
				{Type: "unknown"},
			}),
			want: chatErrCodeUnsupportedPartType,
		},
		{
			name: "binary data empty",
			raw: mustMarshalJSON(t, []RunAgentInputContentPart{
				{Type: "binary", MimeType: "image/png"},
			}),
			want: chatErrCodeBinaryDataEmpty,
		},
		{
			name: "binary mimeType required",
			raw: mustMarshalJSON(t, []RunAgentInputContentPart{
				{Type: "binary", Data: "aGVsbG8="},
			}),
			want: chatErrCodeBinaryMIMERequired,
		},
		{
			name: "binary mimeType unsupported",
			raw: mustMarshalJSON(t, []RunAgentInputContentPart{
				{Type: "binary", MimeType: "image/svg+xml", Data: "aGVsbG8="},
			}),
			want: chatErrCodeBinaryMIMEUnsupported,
		},
		{
			name: "invalid data url",
			raw: mustMarshalJSON(t, []RunAgentInputContentPart{
				{Type: "binary", Data: "data:image/png;base64"},
			}),
			want: chatErrCodeInvalidContentFormat,
		},
		{
			name: "binary decode failed",
			raw: mustMarshalJSON(t, []RunAgentInputContentPart{
				{Type: "binary", MimeType: "image/png", Data: "%%%"},
			}),
			want: chatErrCodeBinaryDecodeFailed,
		},
		{
			name: "binary part too large",
			raw: mustMarshalJSON(t, []RunAgentInputContentPart{
				{Type: "binary", MimeType: "image/png", Data: oversizedData},
			}),
			want: chatErrCodeBinaryPartTooLarge,
		},
		{
			name: "binary total too large",
			raw: mustMarshalJSON(t, []RunAgentInputContentPart{
				{Type: "binary", MimeType: "image/png", Data: totalPartData},
				{Type: "binary", MimeType: "image/png", Data: totalPartData},
				{Type: "binary", MimeType: "image/png", Data: totalPartData},
			}),
			want: chatErrCodeBinaryTotalTooLarge,
		},
		{
			name: "binary parts too many",
			raw: mustMarshalJSON(t, []RunAgentInputContentPart{
				{Type: "binary", MimeType: "image/png", Data: "aGVsbG8="},
				{Type: "binary", MimeType: "image/png", Data: "aGVsbG8="},
				{Type: "binary", MimeType: "image/png", Data: "aGVsbG8="},
				{Type: "binary", MimeType: "image/png", Data: "aGVsbG8="},
				{Type: "binary", MimeType: "image/png", Data: "aGVsbG8="},
			}),
			want: chatErrCodeBinaryPartTooMany,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := parseRunAgentContent(tc.raw)
			assertChatInputErrorCode(t, err, tc.want)
		})
	}
}

func TestChatHandler_InvalidContentReturnsCode(t *testing.T) {
	handler := NewChatHandler(noopChatService{})
	body := `{"messages":[{"role":"user","content":[{"type":"unknown"}]}]}`

	req := httptest.NewRequest(http.MethodPost, "/chat", bytes.NewBufferString(body))
	recorder := httptest.NewRecorder()

	handler.chat(recorder, req)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("unexpected status: %d", recorder.Code)
	}

	var resp map[string]string
	if err := json.Unmarshal(recorder.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp["code"] != chatErrCodeUnsupportedPartType {
		t.Fatalf("unexpected error code: %q", resp["code"])
	}
}

func TestChatHandler_InvalidBodyReturnsCode(t *testing.T) {
	handler := NewChatHandler(noopChatService{})
	req := httptest.NewRequest(http.MethodPost, "/chat", bytes.NewBufferString("{"))
	recorder := httptest.NewRecorder()

	handler.chat(recorder, req)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("unexpected status: %d", recorder.Code)
	}

	var resp map[string]string
	if err := json.Unmarshal(recorder.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp["code"] != chatErrCodeInvalidRequestBody {
		t.Fatalf("unexpected error code: %q", resp["code"])
	}
}

func mustMarshalJSON(t *testing.T, v any) json.RawMessage {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("failed to marshal json: %v", err)
	}
	return data
}

func assertChatInputErrorCode(t *testing.T, err error, wantCode string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error with code %q, got nil", wantCode)
	}
	var inputErr *chatInputError
	if !errors.As(err, &inputErr) {
		t.Fatalf("expected chatInputError, got: %T (%v)", err, err)
	}
	if inputErr.code != wantCode {
		t.Fatalf("unexpected error code: got %q, want %q", inputErr.code, wantCode)
	}
}

type noopChatService struct{}

func (noopChatService) Chat(context.Context, *ChatRequest) (*ChatResponse, error) {
	return nil, nil
}

func (noopChatService) ChatStream(context.Context, *ChatRequest, StreamStartCallback, StreamChunkCallback) error {
	return nil
}

func (noopChatService) ListSessions(context.Context) ([]SessionInfo, error) {
	return nil, nil
}

func (noopChatService) GetSession(context.Context, string) (*GetSessionResponse, error) {
	return nil, nil
}
