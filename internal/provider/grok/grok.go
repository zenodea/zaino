package grok

import (
	"github.com/zenodea/zaino/internal/provider/openaicompat"
)

const (
	ModelGrok4     = "grok-4"
	ModelGrok4Fast = "grok-4-fast"
	ModelGrok3     = "grok-3"
	ModelGrok3Mini = "grok-3-mini"
)

func Config() openaicompat.Config {
	return openaicompat.Config{
		Name:            "grok",
		BaseURL:         "https://api.x.ai/v1",
		DefaultModel:    ModelGrok4,
		KnownModels:     []string{ModelGrok4, ModelGrok4Fast, ModelGrok3, ModelGrok3Mini},
		APIKeyEnv:       []string{"XAI_API_KEY", "GROK_API_KEY"},
		BaseURLEnv:      "XAI_BASE_URL",
		LegacyMaxTokens: true,
		ReasoningEffort: true,
	}
}

func New(opts ...openaicompat.Option) (*openaicompat.Client, error) {
	return openaicompat.New(Config(), opts...)
}
