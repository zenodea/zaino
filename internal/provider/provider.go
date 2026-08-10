package provider

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/zenodea/zaino/internal/anthropic"
	"github.com/zenodea/zaino/internal/gemini"
	"github.com/zenodea/zaino/internal/llm"
)

var constructors = map[string]func() (llm.Provider, error){
	"anthropic": func() (llm.Provider, error) { return anthropic.New() },
	"claude":    func() (llm.Provider, error) { return anthropic.New() },
	"gemini":    func() (llm.Provider, error) { return gemini.New() },
	"google":    func() (llm.Provider, error) { return gemini.New() },
}

var autoOrder = []string{"anthropic", "gemini"}

var ErrNoCredentials = errors.New("provider: no credentials for any provider")

func New(name string) (llm.Provider, error) {
	name = strings.ToLower(strings.TrimSpace(name))

	if name == "" || name == "auto" {
		var missing []error
		for _, candidate := range autoOrder {
			p, err := constructors[candidate]()
			if err == nil {
				return p, nil
			}
			missing = append(missing, err)
		}
		return nil, fmt.Errorf("%w:\n  %s", ErrNoCredentials, joinErrors(missing))
	}

	build, ok := constructors[name]
	if !ok {
		return nil, fmt.Errorf("provider: unknown provider %q (available: %s)",
			name, strings.Join(Available(), ", "))
	}
	return build()
}

func Available() []string {
	names := append([]string(nil), autoOrder...)
	sort.Strings(names)
	return names
}

func joinErrors(errs []error) string {
	msgs := make([]string, len(errs))
	for i, err := range errs {
		msgs[i] = err.Error()
	}
	return strings.Join(msgs, "\n  ")
}
