package openairesponse

import (
	"context"
	"testing"

	"github.com/cloudwego/eino/schema"
)

// TestOldModelsCompatibility 测试旧模型在 Responses API 上的兼容性
func TestOldModelsCompatibility(t *testing.T) {
	ctx := context.Background()

	models := []string{
		"gpt-4o-mini",     // 较新模型
		"gpt-4o",          // 较新模型
		"gpt-4-turbo",     // 旧模型
		"gpt-3.5-turbo",   // 旧模型
	}

	for _, modelName := range models {
		t.Run(modelName, func(t *testing.T) {
			client, err := NewChatModel(ctx, &Config{
				APIKey:  testAPIKey,
				BaseURL: testBaseURL,
				Model:   modelName,
			})
			if err != nil {
				t.Fatalf("failed to create client: %v", err)
			}

			messages := []*schema.Message{
				{
					Role:    schema.User,
					Content: "Say 'hello' in one word.",
				},
			}

			msg, err := client.Generate(ctx, messages)
			if err != nil {
				t.Logf("❌ %s: FAILED - %v", modelName, err)
				return
			}

			t.Logf("✅ %s: %q", modelName, msg.Content)
		})
	}
}
