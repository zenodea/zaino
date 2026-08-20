package attach

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/zenodea/zaino/internal/llm"
)

// Mentions is what a prompt points at that is a picture: @shot.png, which is
// a dropped path with an @ in front of it. A prompt merely talking about a
// file is still only talking.
func Mentions(text string) []string {
	var out []string
	for _, name := range mentioned(text) {
		if IsImage(name) {
			out = append(out, name)
		}
	}
	return out
}

func mentioned(text string) []string {
	var out []string
	for _, field := range strings.Fields(text) {
		name, ok := strings.CutPrefix(field, "@")
		if !ok {
			continue
		}
		if name = strings.Trim(name, `.,;:!?"')`); name != "" {
			out = append(out, name)
		}
	}
	return out
}

// Prompt carries what the text points at. A picture that will not load is
// an error rather than a quiet omission: a prompt about a screenshot that
// arrives without one reads as though the model went blind. Any other name
// is a file only if there is one: @notes.md comes along, @someone is a word.
func Prompt(dir, text string) (llm.Content, []string, error) {
	content := llm.Content{llm.TextBlock{Text: text}}

	var loaded []string
	for _, name := range mentioned(text) {
		path := name
		if !filepath.IsAbs(path) {
			path = filepath.Join(dir, path)
		}
		if IsImage(name) {
			image, err := Image(path)
			if err != nil {
				return nil, nil, err
			}
			content = append(content, image)
			loaded = append(loaded, Describe(name, image))
			continue
		}

		block, what, ok, err := textFile(path, name)
		if err != nil {
			return nil, nil, err
		}
		if !ok {
			continue
		}
		content = append(content, block)
		loaded = append(loaded, what)
	}
	return content, loaded, nil
}

// MaxTextBytes is the most a mentioned file may hold. Past it the read tool,
// with its offsets, is the right way in.
const MaxTextBytes = 256 << 10

func textFile(path, name string) (llm.TextBlock, string, bool, error) {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return llm.TextBlock{}, "", false, nil
	}
	if info.Size() > MaxTextBytes {
		return llm.TextBlock{}, "", false, fmt.Errorf("%s is %s — the ceiling for a mentioned file is %s; ask for it to be read instead",
			filepath.Base(name), Size(info.Size()), Size(MaxTextBytes))
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return llm.TextBlock{}, "", false, err
	}
	if !utf8.Valid(data) {
		return llm.TextBlock{}, "", false, fmt.Errorf("%s is not text", filepath.Base(name))
	}

	body := strings.TrimRight(string(data), "\n")
	lang := strings.TrimPrefix(filepath.Ext(name), ".")
	block := llm.TextBlock{Text: fmt.Sprintf("\n⧉ %s\n```%s\n%s\n```", name, lang, body)}
	lines := strings.Count(body, "\n") + 1
	return block, fmt.Sprintf("%s · %d lines · %s", filepath.Base(name), lines, Size(info.Size())), true, nil
}
