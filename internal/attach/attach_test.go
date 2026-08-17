package attach

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zenodea/zaino/internal/llm"
)

func pngFile(t *testing.T, dir, name string, w, h int) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestMentions(t *testing.T) {
	tests := []struct {
		in   string
		want []string
	}{
		{"look at @shot.png", []string{"shot.png"}},
		{"@a.png and @b.jpeg", []string{"a.png", "b.jpeg"}},
		{"see @out/shot.png, please", []string{"out/shot.png"}},
		{"shot.png is broken", nil},
		{"ask @someone about it", nil},
		{"read @notes.md first", nil},
	}

	for _, tt := range tests {
		got := Mentions(tt.in)
		if strings.Join(got, ",") != strings.Join(tt.want, ",") {
			t.Errorf("Mentions(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestPromptCarriesWhatItPointsAt(t *testing.T) {
	dir := t.TempDir()
	pngFile(t, dir, "shot.png", 4, 2)

	content, attached, err := Prompt(dir, "why is @shot.png wrong")
	if err != nil {
		t.Fatal(err)
	}
	if len(content) != 2 {
		t.Fatalf("content = %+v, want the text and the image", content)
	}
	if text, ok := content[0].(llm.TextBlock); !ok || text.Text != "why is @shot.png wrong" {
		t.Errorf("content[0] = %+v, want the prompt as typed", content[0])
	}
	image, ok := content[1].(llm.ImageBlock)
	if !ok || image.MediaType != "image/png" || len(image.Data) == 0 {
		t.Fatalf("content[1] = %+v", content[1])
	}
	if len(attached) != 1 || !strings.Contains(attached[0], "4×2") {
		t.Errorf("attached = %v, want the size read off the picture", attached)
	}
}

// Going without would read as though the model had gone blind.
func TestAMentionThatWillNotLoadIsAnError(t *testing.T) {
	dir := t.TempDir()

	_, _, err := Prompt(dir, "look at @missing.png")
	if err == nil {
		t.Fatal("got nil, want an error")
	}
	if !strings.Contains(err.Error(), "missing.png") {
		t.Errorf("error does not name it: %v", err)
	}
}

func TestPlainPromptsAreLeftAlone(t *testing.T) {
	content, attached, err := Prompt(t.TempDir(), "nothing to see here")
	if err != nil {
		t.Fatal(err)
	}
	if len(content) != 1 || len(attached) != 0 {
		t.Errorf("content = %+v, attached = %v", content, attached)
	}
}

func TestTheCeilingIsEnforced(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "huge.png")
	if err := os.WriteFile(path, make([]byte, MaxBytes+1), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := Image(path)
	if err == nil || !strings.Contains(err.Error(), "ceiling") {
		t.Errorf("err = %v, want the ceiling named", err)
	}
}

func TestOnlyTheKindsThatTravel(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "notes.md")
	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := Image(path); err == nil {
		t.Fatal("a markdown file was accepted as an image")
	}
	if IsImage(path) {
		t.Error("IsImage said yes to markdown")
	}
	for _, ext := range Kinds() {
		if !IsImage("shot" + ext) {
			t.Errorf("IsImage said no to %s", ext)
		}
	}
}

func TestSize(t *testing.T) {
	cases := map[int64]string{500: "500 bytes", 2048: "2 KB", 3 << 20: "3.0 MB"}
	for n, want := range cases {
		if got := Size(n); got != want {
			t.Errorf("Size(%d) = %q, want %q", n, got, want)
		}
	}
}
