package openaicompat

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
)

const maxModelsBody = 4 << 20

type modelList struct {
	Data []struct {
		ID string `json:"id"`
	} `json:"data"`
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
	for _, m := range list.Data {
		if m.ID != "" {
			out = append(out, m.ID)
		}
	}
	sort.Strings(out)
	return out, nil
}
