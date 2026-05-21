// Package config provides configuration for the imago application.
package config

import (
	"os"
)

// Provider identifies the LLM backend.
type Provider string

const (
	ProviderOpenAI     Provider = "openai"
	ProviderCloudflare Provider = "cloudflare"
	ProviderAnthropic  Provider = "anthropic"
)

// ModelConfig holds model and inference settings for both providers.
type ModelConfig struct {
	Provider         Provider
	InterviewModel   string
	DraftModel       string
	DraftProvider    Provider // if set, draft/revision phases use a different provider
	InterviewOptions map[string]any
	DraftOptions     map[string]any
	RevisionOptions  map[string]any
	MaxTokens        int
	DraftMaxTokens   int
}

// DefaultModelConfig returns the config for the active provider,
// selected by whether Cloudflare env vars are set.
func DefaultModelConfig() ModelConfig {
	if os.Getenv("CLOUDFLARE_ACCOUNT_ID") != "" && os.Getenv("CLOUDFLARE_AXON_GATE_TOKEN") != "" {
		return CloudflareModelConfig()
	}
	return OpenAIModelConfig()
}

// OpenAIModelConfig returns settings for a local OpenAI-compatible server (e.g. llama-server).
func OpenAIModelConfig() ModelConfig {
	return ModelConfig{
		Provider:       ProviderOpenAI,
		InterviewModel: "qwen3:32b",
		DraftModel:     "qwen3:32b",
		InterviewOptions: map[string]any{
			"num_ctx":     8192,
			"num_predict": 2048,
		},
		DraftOptions: map[string]any{
			"num_ctx": 16384,
		},
		RevisionOptions: map[string]any{
			"num_ctx":     16384,
			"num_predict": 4096,
		},
		MaxTokens:      28000,
		DraftMaxTokens: 28000,
	}
}

// CloudflareModelConfig returns settings for Cloudflare Workers AI.
func CloudflareModelConfig() ModelConfig {
	return ModelConfig{
		Provider:         ProviderCloudflare,
		InterviewModel:   "@cf/qwen/qwen3-30b-a3b-fp8",
		DraftModel:       "@cf/qwen/qwen3-30b-a3b-fp8",
		InterviewOptions: map[string]any{"max_tokens": 4096},
		DraftOptions:     map[string]any{"max_tokens": 8192},
		RevisionOptions:  map[string]any{"max_tokens": 8192},
		MaxTokens:        28000,
		DraftMaxTokens:   28000,
	}
}
