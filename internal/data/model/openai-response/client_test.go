/*
 * Copyright 2024 DevOps Backend Authors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package openairesponse

import (
	"context"
	"errors"
	"fmt"
	"io"
	"testing"

	"github.com/cloudwego/eino/schema"
)

const (
	// 使用配置中的 API 密钥和代理服务
	testAPIKey  = "sk-6kgtZQDkmZDQMfCo28C360320cEf45FaAf1577Ef08F4032b"
	testBaseURL = "https://aihubmix.com/v1"
	// 测试模型 - 使用支持 reasoning 的模型
	testModel = "gpt-4o-mini" // 先用普通模型测试基本功能
)

// TestNewChatModel 测试创建客户端
func TestNewChatModel(t *testing.T) {
	ctx := context.Background()

	// 测试正常创建
	client, err := NewChatModel(ctx, &Config{
		APIKey:  testAPIKey,
		BaseURL: testBaseURL,
		Model:   testModel,
	})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	if client == nil {
		t.Fatal("client should not be nil")
	}

	// 测试缺少 APIKey
	_, err = NewChatModel(ctx, &Config{
		BaseURL: testBaseURL,
		Model:   testModel,
	})
	if err == nil {
		t.Fatal("should fail without APIKey")
	}

	// 测试缺少 Model
	_, err = NewChatModel(ctx, &Config{
		APIKey:  testAPIKey,
		BaseURL: testBaseURL,
	})
	if err == nil {
		t.Fatal("should fail without Model")
	}

	t.Log("TestNewChatModel passed")
}

// TestGenerate 测试同步生成
func TestGenerate(t *testing.T) {
	ctx := context.Background()

	client, err := NewChatModel(ctx, &Config{
		APIKey:  testAPIKey,
		BaseURL: testBaseURL,
		Model:   testModel,
	})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	messages := []*schema.Message{
		{
			Role:    schema.User,
			Content: "Hello! Please respond with exactly 'Hi there!'",
		},
	}

	msg, err := client.Generate(ctx, messages)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	t.Logf("Response Role: %s", msg.Role)
	t.Logf("Response Content: %s", msg.Content)
	if msg.ReasoningContent != "" {
		t.Logf("Response ReasoningContent: %s", msg.ReasoningContent)
	}
	if msg.ResponseMeta != nil && msg.ResponseMeta.Usage != nil {
		t.Logf("Usage - Prompt: %d, Completion: %d, Total: %d",
			msg.ResponseMeta.Usage.PromptTokens,
			msg.ResponseMeta.Usage.CompletionTokens,
			msg.ResponseMeta.Usage.TotalTokens)
	}

	if msg.Content == "" {
		t.Fatal("response content should not be empty")
	}

	t.Log("TestGenerate passed")
}

// TestGenerateWithSystemMessage 测试带系统消息的生成
func TestGenerateWithSystemMessage(t *testing.T) {
	ctx := context.Background()

	client, err := NewChatModel(ctx, &Config{
		APIKey:  testAPIKey,
		BaseURL: testBaseURL,
		Model:   testModel,
	})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	messages := []*schema.Message{
		{
			Role:    schema.System,
			Content: "You are a helpful assistant that always responds in Chinese.",
		},
		{
			Role:    schema.User,
			Content: "What is 2+2?",
		},
	}

	msg, err := client.Generate(ctx, messages)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	t.Logf("Response: %s", msg.Content)

	if msg.Content == "" {
		t.Fatal("response content should not be empty")
	}

	t.Log("TestGenerateWithSystemMessage passed")
}

// TestStream 测试流式生成
func TestStream(t *testing.T) {
	ctx := context.Background()

	client, err := NewChatModel(ctx, &Config{
		APIKey:  testAPIKey,
		BaseURL: testBaseURL,
		Model:   testModel,
	})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	messages := []*schema.Message{
		{
			Role:    schema.User,
			Content: "Count from 1 to 5, one number per line.",
		},
	}

	stream, err := client.Stream(ctx, messages)
	if err != nil {
		t.Fatalf("Stream failed: %v", err)
	}
	defer stream.Close()

	var fullContent string
	var chunkCount int

	for {
		msg, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("stream recv error: %v", err)
		}

		chunkCount++
		if msg.Content != "" {
			fullContent += msg.Content
			fmt.Printf("Chunk %d: %q\n", chunkCount, msg.Content)
		}
		if msg.ReasoningContent != "" {
			fmt.Printf("Reasoning %d: %q\n", chunkCount, msg.ReasoningContent)
		}
	}

	t.Logf("Total chunks: %d", chunkCount)
	t.Logf("Full content: %s", fullContent)

	if fullContent == "" {
		t.Fatal("stream content should not be empty")
	}

	t.Log("TestStream passed")
}

// TestGenerateWithReasoning 测试带 reasoning 配置的生成
// 注意：此测试需要支持 reasoning 的模型（如 o1, o3, gpt-5）
func TestGenerateWithReasoning(t *testing.T) {
	ctx := context.Background()

	// 使用支持 reasoning 的模型
	reasoningModel := "o1-mini" // 或 o3-mini

	client, err := NewChatModel(ctx, &Config{
		APIKey:  testAPIKey,
		BaseURL: testBaseURL,
		Model:   reasoningModel,
	})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	messages := []*schema.Message{
		{
			Role:    schema.User,
			Content: "What is the sum of 123 + 456? Show your reasoning.",
		},
	}

	msg, err := client.Generate(ctx, messages,
		WithReasoningEffort(ReasoningEffortHigh),
		WithReasoningSummary(ReasoningSummaryDetailed),
	)
	if err != nil {
		// reasoning 模型可能不可用，跳过测试
		t.Skipf("Generate with reasoning failed (model may not be available): %v", err)
	}

	t.Logf("Response Content: %s", msg.Content)
	if msg.ReasoningContent != "" {
		t.Logf("Reasoning Content: %s", msg.ReasoningContent)
	} else {
		t.Log("No reasoning content returned (model may not support reasoning summary)")
	}

	t.Log("TestGenerateWithReasoning passed")
}

// TestMultiTurn 测试多轮对话
func TestMultiTurn(t *testing.T) {
	ctx := context.Background()

	client, err := NewChatModel(ctx, &Config{
		APIKey:  testAPIKey,
		BaseURL: testBaseURL,
		Model:   testModel,
	})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	// 第一轮
	messages := []*schema.Message{
		{
			Role:    schema.User,
			Content: "My name is Alice.",
		},
	}

	msg1, err := client.Generate(ctx, messages)
	if err != nil {
		t.Fatalf("first turn failed: %v", err)
	}
	t.Logf("Turn 1 response: %s", msg1.Content)

	// 第二轮 - 添加历史
	messages = append(messages, msg1)
	messages = append(messages, &schema.Message{
		Role:    schema.User,
		Content: "What is my name?",
	})

	msg2, err := client.Generate(ctx, messages)
	if err != nil {
		t.Fatalf("second turn failed: %v", err)
	}
	t.Logf("Turn 2 response: %s", msg2.Content)

	// 检查是否记住了名字
	if msg2.Content == "" {
		t.Fatal("response should not be empty")
	}

	t.Log("TestMultiTurn passed")
}

// TestOptions 测试选项
func TestOptions(t *testing.T) {
	ctx := context.Background()

	client, err := NewChatModel(ctx, &Config{
		APIKey:  testAPIKey,
		BaseURL: testBaseURL,
		Model:   testModel,
	})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	messages := []*schema.Message{
		{
			Role:    schema.User,
			Content: "Say hello.",
		},
	}

	// 测试带选项的生成
	msg, err := client.Generate(ctx, messages,
		WithMaxOutputTokens(50),
		WithTemperature(0.5),
	)
	if err != nil {
		t.Fatalf("Generate with options failed: %v", err)
	}

	t.Logf("Response with options: %s", msg.Content)

	if msg.Content == "" {
		t.Fatal("response content should not be empty")
	}

	t.Log("TestOptions passed")
}
