package data

import (
	"testing"

	"devops-backend/internal/conf"
)

func TestResolveClient_Llama4Maverick_UsesRemoteOpenAI(t *testing.T) {
	f := NewClientFactory(conf.Eino{
		Clients: map[string]conf.Client{
			"openai":  {BaseURL: "https://example.com/v1", APIKey: "x"},
			"gemini":  {BaseURL: "https://example.com/gemini", APIKey: "y"},
			"claude":  {BaseURL: "https://example.com", APIKey: "z"},
			"openrouter": {BaseURL: "https://example.com/v1", APIKey: "r"},
		},
	}).(*ClientFactory)

	got := f.ResolveClient("llama-4-maverick", "")
	if got != "openai" {
		t.Fatalf("expected openai, got %q", got)
	}
}
