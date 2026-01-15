package biz

import (
	"context"

	"github.com/cloudwego/eino/components/model"
)

// RequestParams 请求参数
// 所有需要被 option 化的参数都放在这里，各 adapter 根据需要提取和转换为具体 client 的配置
type RequestParams struct {
	// Thinking 思考模式
	// nil: 使用默认行为
	// true: 启用思考模式
	// false: 禁用思考模式
	Thinking *bool

	// 未来可以添加更多参数，例如：
	// Temperature *float64
	// TopP *float64
	// MaxTokens *int
}

// WithParams 创建请求参数选项
func WithParams(params *RequestParams) model.Option {
	return model.WrapImplSpecificOptFn(func(p *RequestParams) {
		if params == nil {
			return
		}
		if params.Thinking != nil {
			p.Thinking = params.Thinking
		}
		// 未来添加更多字段时在这里复制
	})
}

// GetParams 从 opts 中提取 RequestParams
func GetParams(opts ...model.Option) *RequestParams {
	return model.GetImplSpecificOptions(&RequestParams{}, opts...)
}

// ChatModelFactory 聊天模型工厂接口
type ChatModelFactory interface {
	// ResolveClient 根据模型名称解析应使用的客户端
	// 优先级：requestClient > 关键词匹配 > openai
	ResolveClient(modelName, requestClient string) string

	// CreateChatModel 根据客户端名称和模型创建 ChatModel
	// useAdapter: true 时使用 adapter 包装（自动处理 thinking 等特性），false 时返回原始模型
	CreateChatModel(ctx context.Context, clientName, modelName string, useAdapter bool) (model.BaseChatModel, error)
}
