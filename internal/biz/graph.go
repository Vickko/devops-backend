package biz

import (
	"context"
	"fmt"

	"devops-backend/internal/conf"

	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

// DebugGraphs 用于构建调试用的示例图
type DebugGraphs struct{}

// NewDebugGraphs 创建并注册所有调试图
func NewDebugGraphs(ctx context.Context, clientFactory ChatModelFactory, cfg conf.Eino) (*DebugGraphs, error) {
	g := &DebugGraphs{}

	if err := g.buildSimpleChatGraph(ctx, clientFactory, cfg); err != nil {
		return nil, fmt.Errorf("build simple chat graph: %w", err)
	}

	return g, nil
}

// buildSimpleChatGraph 构建简单聊天图
func (g *DebugGraphs) buildSimpleChatGraph(ctx context.Context, clientFactory ChatModelFactory, cfg conf.Eino) error {
	var messageHistory []*schema.Message

	graph := compose.NewGraph[*schema.Message, *schema.Message]()

	// Lambda 节点：维护消息列表
	lambda := compose.InvokableLambda(func(ctx context.Context, userMsg *schema.Message) ([]*schema.Message, error) {
		messageHistory = append(messageHistory, userMsg)

		systemPrompt := &schema.Message{
			Role:    schema.System,
			Content: "你是一个友好的AI助手，请用简洁明了的方式回答用户的问题。",
		}
		return append([]*schema.Message{systemPrompt}, messageHistory...), nil
	})
	if err := graph.AddLambdaNode("message_manager", lambda); err != nil {
		return fmt.Errorf("add lambda node: %w", err)
	}

	// ChatModel 节点
	chatModel, err := NewChatModel(ctx, clientFactory, cfg)
	if err != nil {
		return fmt.Errorf("create chat model: %w", err)
	}
	if err := graph.AddChatModelNode("chat_model", chatModel); err != nil {
		return fmt.Errorf("add chat model node: %w", err)
	}

	// 连接节点
	if err := graph.AddEdge(compose.START, "message_manager"); err != nil {
		return fmt.Errorf("add edge START -> message_manager: %w", err)
	}
	if err := graph.AddEdge("message_manager", "chat_model"); err != nil {
		return fmt.Errorf("add edge message_manager -> chat_model: %w", err)
	}
	if err := graph.AddEdge("chat_model", compose.END); err != nil {
		return fmt.Errorf("add edge chat_model -> END: %w", err)
	}

	// 编译
	_, err = graph.Compile(ctx, compose.WithGraphName("simple_chat"))
	if err != nil {
		return fmt.Errorf("compile graph: %w", err)
	}

	return nil
}
