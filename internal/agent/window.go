package agent

import (
	"strings"
	"sync"
)

var windows = struct {
	mu    sync.Mutex
	known map[string]int
}{known: map[string]int{
	"claude-": 200_000,
	"gemini-": 1_000_000,
	"gpt-4.1": 1_000_000,
	"gpt-5":   400_000,
	"o3":      200_000,
	"o4":      200_000,
	"grok-4":  256_000,
	"grok-3":  131_072,
}}

func SetWindow(model string, tokens int) {
	windows.mu.Lock()
	defer windows.mu.Unlock()
	windows.known[strings.ToLower(model)] = tokens
}

// WindowFor takes the exact id, then the longest known prefix, the vendor
// path stripped; 0 when nothing matches.
func WindowFor(model string) int {
	windows.mu.Lock()
	defer windows.mu.Unlock()
	model = strings.ToLower(model)
	if n, ok := windows.known[model]; ok {
		return n
	}
	if _, after, ok := strings.Cut(model, "/"); ok {
		if n, ok := windows.known[after]; ok {
			return n
		}
		model = after
	}
	best, found := "", 0
	for id, n := range windows.known {
		if strings.HasPrefix(model, id) && len(id) > len(best) {
			best, found = id, n
		}
	}
	return found
}

func (a *Agent) modelID() string {
	if a.Model != "" {
		return a.Model
	}
	if a.Provider != nil {
		return a.Provider.DefaultModel()
	}
	return ""
}
