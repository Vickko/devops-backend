package message

import (
	"encoding/json"
	"os"
	"sync"
)

// ModalityType 模态类型
type ModalityType string

const (
	ModalityText  ModalityType = "text"
	ModalityImage ModalityType = "image"
	ModalityAudio ModalityType = "audio"
	ModalityVideo ModalityType = "video"
)

// ModelCapabilities 模型能力定义
type ModelCapabilities struct {
	SupportedModalities  map[ModalityType]bool `json:"supported_modalities"`
	RequiresNonStreaming bool                  `json:"requires_non_streaming"` // 是否需要 fallback 到非流式模式
}

// ModelCapabilityRegistry 模型能力注册表
type ModelCapabilityRegistry struct {
	mu           sync.RWMutex
	capabilities map[string]*ModelCapabilities
}

var (
	globalRegistry     *ModelCapabilityRegistry
	registryInitOnce   sync.Once
	capabilitiesConfig = "configs/model_capabilities.json" // 可配置的能力文件路径
)

// GetModelCapabilityRegistry 获取全局模型能力注册表
func GetModelCapabilityRegistry() *ModelCapabilityRegistry {
	registryInitOnce.Do(func() {
		globalRegistry = newModelCapabilityRegistry()
		globalRegistry.loadHardcodedCapabilities()
		globalRegistry.loadFromConfig(capabilitiesConfig)
	})
	return globalRegistry
}

// newModelCapabilityRegistry 创建新的模型能力注册表
func newModelCapabilityRegistry() *ModelCapabilityRegistry {
	return &ModelCapabilityRegistry{
		capabilities: make(map[string]*ModelCapabilities),
	}
}

// loadHardcodedCapabilities 加载硬编码的模型能力
func (r *ModelCapabilityRegistry) loadHardcodedCapabilities() {
	r.mu.Lock()
	defer r.mu.Unlock()

	// DeepSeek 系列：仅支持文本
	r.capabilities["deepseek"] = &ModelCapabilities{
		SupportedModalities: map[ModalityType]bool{
			ModalityText:  true,
			ModalityImage: false,
			ModalityAudio: false,
			ModalityVideo: false,
		},
		RequiresNonStreaming: false,
	}

	// 图像生成模型：需要 fallback 到非流式模式
	// 这些模型的流式接口可能不稳定或不支持，需要使用 Generate() 然后模拟流式
	imageGenerationModels := []string{
		"gemini-3-pro-image-preview",
		"gemini-2.5-flash-image",
	}

	for _, model := range imageGenerationModels {
		r.capabilities[model] = &ModelCapabilities{
			SupportedModalities: map[ModalityType]bool{
				ModalityText:  true,
				ModalityImage: true,
				ModalityAudio: true,
				ModalityVideo: true,
			},
			RequiresNonStreaming: true, // 关键：需要 fallback
		}
	}

	// 可以继续添加其他已知的能力受限模型
	// 例如：某些早期的 GPT 模型可能不支持图像等
}

// loadFromConfig 从配置文件加载模型能力
func (r *ModelCapabilityRegistry) loadFromConfig(configPath string) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		// 配置文件不存在或读取失败，跳过
		return
	}

	var externalCaps map[string]*ModelCapabilities
	if err := json.Unmarshal(data, &externalCaps); err != nil {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// 合并外部配置，外部配置优先级更高
	for modelName, caps := range externalCaps {
		r.capabilities[modelName] = caps
	}
}

// GetCapabilities 获取指定模型的能力
// 如果模型不在注册表中，返回 nil（表示默认支持所有模态）
func (r *ModelCapabilityRegistry) GetCapabilities(clientName string) *ModelCapabilities {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.capabilities[clientName]
}

// SupportsModality 检查模型是否支持指定模态
// 如果模型不在注册表中，默认返回 true（支持所有模态）
func (r *ModelCapabilityRegistry) SupportsModality(clientName string, modality ModalityType) bool {
	caps := r.GetCapabilities(clientName)
	if caps == nil {
		// 不在注册表中的模型，默认支持所有模态
		return true
	}
	return caps.SupportedModalities[modality]
}

// RequiresNonStreamingMode 检查模型是否需要 fallback 到非流式模式
// 如果模型不在注册表中，默认返回 false（支持流式）
func (r *ModelCapabilityRegistry) RequiresNonStreamingMode(modelName string) bool {
	caps := r.GetCapabilities(modelName)
	if caps == nil {
		// 不在注册表中的模型，默认支持流式
		return false
	}
	return caps.RequiresNonStreaming
}
