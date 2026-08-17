package tool

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zenodea/zaino/internal/llm"
)

func writePNG(t *testing.T, w *Workspace, name string, width, height int) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	img.Set(0, 0, color.RGBA{B: 255, A: 255})

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(w.Root, name)
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestReadHandsBackAPicture(t *testing.T) {
	w := workspace(t)
	writePNG(t, w, "shot.png", 8, 4)

	call, err := prepare(t, &Read{w}, map[string]any{"path": "shot.png"})
	if err != nil {
		t.Fatal(err)
	}
	out, err := call.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	// The text says what it is; the picture itself rides alongside.
	for _, want := range []string{"shot.png", "image/png", "8×4"} {
		if !strings.Contains(out, want) {
			t.Errorf("result %q does not mention %q", out, want)
		}
	}

	with, ok := call.(Attaches)
	if !ok {
		t.Fatal("a read of an image attached nothing")
	}
	attached := with.Attachments()
	if len(attached) != 1 {
		t.Fatalf("attachments = %+v", attached)
	}
	if block, ok := attached[0].(llm.ImageBlock); !ok || block.MediaType != "image/png" {
		t.Errorf("attachment = %+v", attached[0])
	}
}

// Reading anything else must go on working exactly as it did.
func TestReadingTextAttachesNothing(t *testing.T) {
	w := workspace(t)
	write(t, w, "notes.md", "hello\n")

	call, err := prepare(t, &Read{w}, map[string]any{"path": "notes.md"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := call.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if with, ok := call.(Attaches); ok && len(with.Attachments()) != 0 {
		t.Errorf("a text file attached %+v", with.Attachments())
	}
}

func TestAnImageTooBigToSendSaysSo(t *testing.T) {
	w := workspace(t)
	path := filepath.Join(w.Root, "huge.png")
	if err := os.WriteFile(path, make([]byte, (5<<20)+1), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := run(t, &Read{w}, map[string]any{"path": "huge.png"})
	if err == nil || !strings.Contains(err.Error(), "ceiling") {
		t.Errorf("err = %v, want the ceiling named", err)
	}
}
