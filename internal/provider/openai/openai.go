package openai

import (
	"github.com/zenodea/zaino/internal/provider/openaicompat"
)

const (
	ModelGPT51     = "gpt-5.1"
	ModelGPT51Mini = "gpt-5.1-mini"
	ModelGPT5      = "gpt-5"
	ModelGPT5Mini  = "gpt-5-mini"
)

func Config() openaicompat.Config {
	return openaicompat.Config{
		Name:            "openai",
		BaseURL:         "https://api.openai.com/v1",
		DefaultModel:    ModelGPT51,
		KnownModels:     []string{ModelGPT51, ModelGPT51Mini, ModelGPT5, ModelGPT5Mini},
		APIKeyEnv:       []string{"OPENAI_API_KEY"},
		BaseURLEnv:      "OPENAI_BASE_URL",
		ReasoningEffort: true,
		DeveloperRole:   true,
	}
}

func New(opts ...openaicompat.Option) (*openaicompat.Client, error) {
	return openaicompat.New(Config(), opts...)
}
