package anthropic

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
)

const maxModelsBody = 4 << 20

// FetchModels walks /v1/models, which is paginated and ordered newest first.
func (c *Client) FetchModels(ctx context.Context) ([]string, error) {
	key, token, err := c.credential(ctx)
	if err != nil {
		return nil, err
	}

	var out []string
	after := ""
	for range 20 {
		endpoint := c.baseURL + "/v1/models?limit=100"
		if after != "" {
			endpoint += "&after_id=" + url.QueryEscape(after)
		}

		resp, err := c.http.Get(ctx, endpoint, c.setRequestHeaders(key, token))
		if err != nil {
			return nil, err
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, maxModelsBody))
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("anthropic: reading models: %w", err)
		}

		var page struct {
			Data []struct {
				ID string `json:"id"`
			} `json:"data"`
			HasMore bool   `json:"has_more"`
			LastID  string `json:"last_id"`
		}
		if err := json.Unmarshal(body, &page); err != nil {
			return nil, fmt.Errorf("anthropic: models: %w", err)
		}
		for _, m := range page.Data {
			if m.ID != "" {
				out = append(out, m.ID)
			}
		}
		if !page.HasMore || page.LastID == "" {
			break
		}
		after = page.LastID
	}
	return out, nil
}
