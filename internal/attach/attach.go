// Package attach turns a file into something the model can look at: an image
// on a prompt you are typing, or on the result of a tool that read one.
package attach

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"path/filepath"
	"strings"

	"github.com/zenodea/zaino/internal/llm"
)

// Anthropic refuses an image over five megabytes and the others are no more
// generous, so the ceiling is theirs rather than ours.
const MaxBytes = 5 << 20

var kinds = map[string]string{
	".png":  "image/png",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".gif":  "image/gif",
	".webp": "image/webp",
}

func IsImage(path string) bool {
	_, ok := kinds[strings.ToLower(filepath.Ext(path))]
	return ok
}

func Image(path string) (llm.ImageBlock, error) {
	kind, ok := kinds[strings.ToLower(filepath.Ext(path))]
	if !ok {
		return llm.ImageBlock{}, fmt.Errorf("%s: not an image zaino can send — %s",
			filepath.Base(path), strings.Join(Kinds(), ", "))
	}

	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return llm.ImageBlock{}, fmt.Errorf("%s does not exist", path)
		}
		if errors.Is(err, os.ErrPermission) {
			return llm.ImageBlock{}, fmt.Errorf("%s is not readable", path)
		}
		return llm.ImageBlock{}, err
	}
	if info.Size() > MaxBytes {
		return llm.ImageBlock{}, fmt.Errorf("%s is %s — the ceiling is %s",
			filepath.Base(path), Size(info.Size()), Size(MaxBytes))
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return llm.ImageBlock{}, err
	}
	return llm.ImageBlock{MediaType: kind, Data: data}, nil
}

func Kinds() []string {
	return []string{".png", ".jpg", ".jpeg", ".gif", ".webp"}
}

func Describe(name string, b llm.ImageBlock) string {
	out := fmt.Sprintf("%s · %s · %s", filepath.Base(name), b.MediaType, Size(int64(len(b.Data))))
	if cfg, _, err := image.DecodeConfig(bytes.NewReader(b.Data)); err == nil {
		out += fmt.Sprintf(" · %d×%d", cfg.Width, cfg.Height)
	}
	return out
}

func Size(n int64) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%d KB", n/(1<<10))
	}
	return fmt.Sprintf("%d bytes", n)
}
