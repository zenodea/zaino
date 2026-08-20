package llm

// Price is what a model charges, in US dollars per million tokens.
type Price struct {
	Input      float64
	Output     float64
	CacheRead  float64
	CacheWrite float64
}

func (p Price) Cost(u Usage) float64 {
	const million = 1_000_000
	return (float64(u.InputTokens)*p.Input +
		float64(u.OutputTokens+u.ThinkingTokens)*p.Output +
		float64(u.CacheReadTokens)*p.CacheRead +
		float64(u.CacheWriteTokens)*p.CacheWrite) / million
}

// PriceLister is optional: a provider whose host publishes what each model
// costs, learned along with the model list.
type PriceLister interface {
	Prices() map[string]Price
}
