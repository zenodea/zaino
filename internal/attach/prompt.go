package attach

import (
	"path/filepath"
	"strings"

	"github.com/zenodea/zaino/internal/llm"
)

// The files a prompt points at: @shot.png, which is a dropped path with an @
// in front of it. A prompt merely talking about a file is still only talking.
func Mentions(text string) []string {
	var out []string
	for _, field := range strings.Fields(text) {
		name, ok := strings.CutPrefix(field, "@")
		if !ok {
			continue
		}
		name = strings.Trim(name, `.,;:!?"')`)
		if name != "" && IsImage(name) {
			out = append(out, name)
		}
	}
	return out
}

// A mention that will not load is an error rather than a quiet omission: a
// prompt about a screenshot that arrives without one reads as though the
// model went blind.
func Prompt(dir, text string) (llm.Content, []string, error) {
	content := llm.Content{llm.TextBlock{Text: text}}

	var loaded []string
	for _, name := range Mentions(text) {
		path := name
		if !filepath.IsAbs(path) {
			path = filepath.Join(dir, path)
		}
		image, err := Image(path)
		if err != nil {
			return nil, nil, err
		}
		content = append(content, image)
		loaded = append(loaded, Describe(name, image))
	}
	return content, loaded, nil
}
