package openaicompat

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/zenodea/zaino/internal/llm"
	"io"
	"sort"
	"strconv"
)

const maxModelsBody = 4 << 20

type modelList struct {
	Data []struct {
		ID string `json:"id"`

		// OpenRouter publishes what each model costs per token, as decimal
		// strings; other hosts leave it out.
		Pricing *struct {
			Prompt     string `json:"prompt"`
			Completion string `json:"completion"`
			CacheRead  string `json:"input_cache_read"`
			CacheWrite string `json:"input_cache_write"`
		} `json:"pricing"`
	} `json:"data"`
}

// Prices is what the last model listing said each model costs, per million
// tokens. Empty until FetchModels has run against a host that publishes it.
func (c *Client) Prices() map[string]llm.Price {
	c.pricesMu.Lock()
	defer c.pricesMu.Unlock()
	out := make(map[string]llm.Price, len(c.prices))
	for id, p := range c.prices {
		out[id] = p
	}
	return out
}

func perMillion(s string) float64 {
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return f * 1_000_000
}

// FetchModels asks the host what this credential can reach. Every
// OpenAI-compatible API serves the same /models shape.
func (c *Client) FetchModels(ctx context.Context) ([]string, error) {
	resp, err := c.http.Get(ctx, c.baseURL+"/models", c.setRequestHeaders)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxModelsBody))
	if err != nil {
		return nil, fmt.Errorf("%s: reading models: %w", c.cfg.Name, err)
	}

	var list modelList
	if err := json.Unmarshal(body, &list); err != nil {
		return nil, fmt.Errorf("%s: models: %w", c.cfg.Name, err)
	}

	out := make([]string, 0, len(list.Data))
	learned := map[string]llm.Price{}
	for _, m := range list.Data {
		if m.ID == "" {
			continue
		}
		out = append(out, m.ID)
		if pr := m.Pricing; pr != nil && (pr.Prompt != "" || pr.Completion != "") {
			learned[m.ID] = llm.Price{
				Input: perMillion(pr.Prompt), Output: perMillion(pr.Completion),
				CacheRead: perMillion(pr.CacheRead), CacheWrite: perMillion(pr.CacheWrite),
			}
		}
	}
	sort.Strings(out)
	if len(learned) > 0 {
		c.pricesMu.Lock()
		c.prices = learned
		c.pricesMu.Unlock()
	}
	return out, nil
}
