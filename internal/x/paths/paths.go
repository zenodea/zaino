package paths

import (
	"os"
	"path/filepath"
	"strings"
)

func Data(elem ...string) (string, error) {
	root := os.Getenv("XDG_DATA_HOME")
	if root == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		root = filepath.Join(home, ".local", "share")
	}

	path := filepath.Join(append([]string{root, "zaino"}, elem...)...)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", err
	}
	return path, nil
}

func Slug(dir string) string {
	dir = filepath.Clean(dir)
	slug := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			return r
		case r == '-', r == '_', r == '.':
			return r
		default:
			return '-'
		}
	}, dir)
	slug = strings.Trim(slug, "-")
	if slug == "" {
		return "root"
	}
	return slug
}
