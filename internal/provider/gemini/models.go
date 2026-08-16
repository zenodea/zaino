package gemini

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"slices"
	"sort"
	"strings"
)

const maxModelsBody = 4 << 20

// FetchModels lists the models this key reaches, keeping only the ones that
// can actually answer a turn.
func (c *Client) FetchModels(ctx context.Context) ([]string, error) {
	var out []string
	token := ""
	for range 20 {
		endpoint := c.baseURL + "/models?pageSize=200"
		if token != "" {
			endpoint += "&pageToken=" + url.QueryEscape(token)
		}

		resp, err := c.http.Get(ctx, endpoint, c.setRequestHeaders)
		if err != nil {
			return nil, err
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, maxModelsBody))
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("gemini: reading models: %w", err)
		}

		var page struct {
			Models []struct {
				Name    string   `json:"name"`
				Methods []string `json:"supportedGenerationMethods"`
			} `json:"models"`
			NextPageToken string `json:"nextPageToken"`
		}
		if err := json.Unmarshal(body, &page); err != nil {
			return nil, fmt.Errorf("gemini: models: %w", err)
		}

		for _, m := range page.Models {
			// Embedding and vision-only models cannot hold a conversation.
			if len(m.Methods) > 0 && !slices.Contains(m.Methods, "generateContent") {
				continue
			}
			if id := strings.TrimPrefix(m.Name, "models/"); id != "" {
				out = append(out, id)
			}
		}
		if page.NextPageToken == "" {
			break
		}
		token = page.NextPageToken
	}
	sort.Strings(out)
	return out, nil
}
