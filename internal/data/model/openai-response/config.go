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

import "net/http"

const (
	defaultBaseURL = "https://api.openai.com/v1"
)

// Config 客户端配置
type Config struct {
	// APIKey OpenAI API 密钥
	// Required
	APIKey string `json:"api_key"`

	// BaseURL API 基础 URL
	// Optional. Default: https://api.openai.com/v1
	BaseURL string `json:"base_url"`

	// Model 模型 ID
	// Required
	Model string `json:"model"`

	// HTTPClient HTTP 客户端
	// Optional. Default: http.DefaultClient
	HTTPClient *http.Client `json:"-"`

	// MaxOutputTokens 最大输出 token 数
	// Optional
	MaxOutputTokens *int `json:"max_output_tokens,omitempty"`

	// Temperature 采样温度 (0-2)
	// Optional. Default: 1
	Temperature *float32 `json:"temperature,omitempty"`

	// TopP 核采样参数
	// Optional. Default: 1
	TopP *float32 `json:"top_p,omitempty"`

	// Store 是否存储响应
	// Optional. Default: true
	Store *bool `json:"store,omitempty"`

	// ReasoningEffort 推理努力程度
	// Optional. Values: low, medium, high
	ReasoningEffort ReasoningEffort `json:"reasoning_effort,omitempty"`

	// ReasoningSummary 推理摘要模式
	// Optional. Values: auto, concise, detailed
	ReasoningSummary ReasoningSummary `json:"reasoning_summary,omitempty"`
}

// getBaseURL 获取 BaseURL，使用默认值
func (c *Config) getBaseURL() string {
	if c.BaseURL == "" {
		return defaultBaseURL
	}
	return c.BaseURL
}

// getHTTPClient 获取 HTTP 客户端，使用默认值
func (c *Config) getHTTPClient() *http.Client {
	if c.HTTPClient == nil {
		return http.DefaultClient
	}
	return c.HTTPClient
}
