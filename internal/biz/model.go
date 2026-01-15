package biz

import (
	"context"

	"devops-backend/internal/conf"

	"github.com/cloudwego/eino/components/model"
)

// NewChatModel 创建聊天模型（使用 openai 客户端）
func NewChatModel(ctx context.Context, clientFactory ChatModelFactory, cfg conf.Eino) (model.BaseChatModel, error) {
	return clientFactory.CreateChatModel(ctx, "openai", cfg.DefaultModel, true)
}
