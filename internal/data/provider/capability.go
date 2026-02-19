package provider

import (
	"encoding/json"
	"os"
	"strings"
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
	RequiresNonStreaming bool                  `json:"requires_non_streaming"`
}

// ModelCapabilityRegistry 模型能力注册表
type ModelCapabilityRegistry struct {
	mu           sync.RWMutex
	capabilities map[string]*ModelCapabilities
}

var (
	globalRegistry     *ModelCapabilityRegistry
	registryInitOnce   sync.Once
	capabilitiesConfig = "configs/model_capabilities.json"
)

// GetModelCapabilityRegistry 获取全局模型能力注册表
func GetModelCapabilityRegistry() *ModelCapabilityRegistry {
	registryInitOnce.Do(func() {
		globalRegistry = &ModelCapabilityRegistry{capabilities: make(map[string]*ModelCapabilities)}
		globalRegistry.loadHardcodedCapabilities()
		globalRegistry.loadFromConfig(capabilitiesConfig)
	})
	return globalRegistry
}

func (r *ModelCapabilityRegistry) loadHardcodedCapabilities() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.capabilities["deepseek"] = &ModelCapabilities{
		SupportedModalities: map[ModalityType]bool{
			ModalityText: true, ModalityImage: false, ModalityAudio: false, ModalityVideo: false,
		},
	}

	for _, m := range []string{"gemini-3-pro-image-preview", "gemini-2.5-flash-image"} {
		r.capabilities[m] = &ModelCapabilities{
			SupportedModalities: map[ModalityType]bool{
				ModalityText: true, ModalityImage: true, ModalityAudio: true, ModalityVideo: true,
			},
			RequiresNonStreaming: true,
		}
	}
}

func (r *ModelCapabilityRegistry) loadFromConfig(configPath string) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return
	}
	var ext map[string]*ModelCapabilities
	if err := json.Unmarshal(data, &ext); err != nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for k, v := range ext {
		r.capabilities[k] = v
	}
}

func (r *ModelCapabilityRegistry) GetCapabilities(name string) *ModelCapabilities {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if caps, ok := r.capabilities[name]; ok {
		return caps
	}

	lowerName := strings.ToLower(strings.TrimSpace(name))
	if lowerName == "" {
		return nil
	}
	for key, caps := range r.capabilities {
		if strings.Contains(lowerName, strings.ToLower(key)) {
			return caps
		}
	}

	return nil
}

func (r *ModelCapabilityRegistry) SupportsModality(name string, modality ModalityType) bool {
	caps := r.GetCapabilities(name)
	if caps == nil {
		return true
	}
	return caps.SupportedModalities[modality]
}

func (r *ModelCapabilityRegistry) RequiresNonStreamingMode(modelName string) bool {
	caps := r.GetCapabilities(modelName)
	if caps == nil {
		return false
	}
	return caps.RequiresNonStreaming
}
