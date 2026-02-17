package provider

import (
	"context"
	"strings"

	"devops-backend/internal/conf"

	"github.com/cloudwego/eino-ext/components/model/arkbot"
	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino-ext/components/model/qianfan"
	"github.com/cloudwego/eino/components/model"
)

// newOpenAICompatible 直通 OpenAI 兼容接口（grok, glm, kimi, minimax, default fallback）
func newOpenAICompatible(ctx context.Context, cfg conf.Client, modelName string, opts ...model.Option) (model.BaseChatModel, error) {
	return openai.NewChatModel(ctx, &openai.ChatModelConfig{
		BaseURL: cfg.BaseURL, APIKey: cfg.APIKey, Model: modelName,
	})
}

func newArkBot(ctx context.Context, cfg conf.Client, modelName string, opts ...model.Option) (model.BaseChatModel, error) {
	return arkbot.NewChatModel(ctx, &arkbot.Config{
		BaseURL: cfg.BaseURL, APIKey: cfg.APIKey, Model: modelName,
	})
}

func newQianfan(ctx context.Context, cfg conf.Client, modelName string, opts ...model.Option) (model.BaseChatModel, error) {
	if cfg.APIKey != "" {
		qfCfg := qianfan.GetQianfanSingletonConfig()
		if parts := strings.SplitN(cfg.APIKey, ":", 2); len(parts) == 2 {
			qfCfg.AccessKey = parts[0]
			qfCfg.SecretKey = parts[1]
		}
	}
	if cfg.BaseURL != "" {
		qianfan.GetQianfanSingletonConfig().BaseURL = cfg.BaseURL
	}
	return qianfan.NewChatModel(ctx, &qianfan.ChatModelConfig{Model: modelName})
}
