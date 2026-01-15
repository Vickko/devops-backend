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
	"github.com/cloudwego/eino/components/model"
)

// responsesOptions 请求选项
type responsesOptions struct {
	ReasoningEffort    ReasoningEffort
	ReasoningSummary   ReasoningSummary
	MaxOutputTokens    *int
	Temperature        *float32
	TopP               *float32
	Store              *bool
	Instructions       string
	PreviousResponseID string
}

// WithReasoningEffort 设置推理努力程度
// Values: low, medium, high
func WithReasoningEffort(effort ReasoningEffort) model.Option {
	return model.WrapImplSpecificOptFn(func(o *responsesOptions) {
		o.ReasoningEffort = effort
	})
}

// WithReasoningSummary 设置推理摘要模式
// Values: auto, concise, detailed
func WithReasoningSummary(summary ReasoningSummary) model.Option {
	return model.WrapImplSpecificOptFn(func(o *responsesOptions) {
		o.ReasoningSummary = summary
	})
}

// WithMaxOutputTokens 设置最大输出 token 数
func WithMaxOutputTokens(tokens int) model.Option {
	return model.WrapImplSpecificOptFn(func(o *responsesOptions) {
		o.MaxOutputTokens = &tokens
	})
}

// WithTemperature 设置采样温度
func WithTemperature(temp float32) model.Option {
	return model.WrapImplSpecificOptFn(func(o *responsesOptions) {
		o.Temperature = &temp
	})
}

// WithTopP 设置核采样参数
func WithTopP(topP float32) model.Option {
	return model.WrapImplSpecificOptFn(func(o *responsesOptions) {
		o.TopP = &topP
	})
}

// WithStore 设置是否存储响应
func WithStore(store bool) model.Option {
	return model.WrapImplSpecificOptFn(func(o *responsesOptions) {
		o.Store = &store
	})
}

// WithInstructions 设置系统指令
func WithInstructions(instructions string) model.Option {
	return model.WrapImplSpecificOptFn(func(o *responsesOptions) {
		o.Instructions = instructions
	})
}

// WithPreviousResponseID 设置上一个响应 ID (用于多轮对话)
func WithPreviousResponseID(id string) model.Option {
	return model.WrapImplSpecificOptFn(func(o *responsesOptions) {
		o.PreviousResponseID = id
	})
}

// getOptions 从 opts 中提取选项
func getOptions(config *Config, opts []model.Option) *responsesOptions {
	defaultOpts := &responsesOptions{
		ReasoningEffort:  config.ReasoningEffort,
		ReasoningSummary: config.ReasoningSummary,
		MaxOutputTokens:  config.MaxOutputTokens,
		Temperature:      config.Temperature,
		TopP:             config.TopP,
		Store:            config.Store,
	}
	return model.GetImplSpecificOptions(defaultOpts, opts...)
}
