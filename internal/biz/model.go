package biz

import (
	"context"

	"devops-backend/internal/conf"

	"github.com/cloudwego/eino/components/model"
)

// NewChatModel 创建聊天模型（使用默认模型）
func NewChatModel(ctx context.Context, provider ChatModelProvider, cfg conf.Eino) (model.ToolCallingChatModel, error) {
	return provider.CreateChatModel(ctx, cfg.DefaultModel)
}
