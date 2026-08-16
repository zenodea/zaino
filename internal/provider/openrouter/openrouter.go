package openrouter

import (
	"github.com/zenodea/zaino/internal/provider/openaicompat"
)

// OpenRouter fronts hundreds of models from every vendor, so there is no
// useful list to compile here — /model asks the API instead. Ids are
// namespaced by vendor: anthropic/claude-opus-4.5, openai/gpt-5.1, and so on.
const (
	ModelAuto = "openrouter/auto"
)

func Config() openaicompat.Config {
	return openaicompat.Config{
		Name:            "openrouter",
		BaseURL:         "https://openrouter.ai/api/v1",
		DefaultModel:    ModelAuto,
		KnownModels:     nil,
		APIKeyEnv:       []string{"OPENROUTER_API_KEY"},
		BaseURLEnv:      "OPENROUTER_BASE_URL",
		LegacyMaxTokens: true,
		ReasoningEffort: true,
	}
}

func New(opts ...openaicompat.Option) (*openaicompat.Client, error) {
	return openaicompat.New(Config(), opts...)
}
