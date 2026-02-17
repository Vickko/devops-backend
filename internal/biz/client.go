package biz

import (
	"context"

	"github.com/cloudwego/eino/components/model"
)

// RequestParams 请求参数
type RequestParams struct {
	Thinking *bool
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
	})
}

// GetParams 从 opts 中提取 RequestParams
func GetParams(opts ...model.Option) *RequestParams {
	return model.GetImplSpecificOptions(&RequestParams{}, opts...)
}

// ChatModelProvider 聊天模型提供者接口
type ChatModelProvider interface {
	CreateChatModel(ctx context.Context, modelName string, opts ...model.Option) (model.ToolCallingChatModel, error)
}
