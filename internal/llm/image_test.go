package llm

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestImageBlockTravelsAsBase64(t *testing.T) {
	raw, err := json.Marshal(ImageBlock{MediaType: "image/png", Data: []byte("not really a png")})
	if err != nil {
		t.Fatal(err)
	}

	var wire struct {
		Type   string `json:"type"`
		Source struct {
			Type      string `json:"type"`
			MediaType string `json:"media_type"`
			Data      string `json:"data"`
		} `json:"source"`
	}
	if err := json.Unmarshal(raw, &wire); err != nil {
		t.Fatal(err)
	}
	if wire.Type != "image" || wire.Source.Type != "base64" || wire.Source.MediaType != "image/png" {
		t.Errorf("wire = %s", raw)
	}
	if strings.Contains(wire.Source.Data, "not really") {
		t.Errorf("the bytes went out unencoded: %s", raw)
	}
}

// A session comes back off disk as JSON, so an image has to survive the trip
// or a resumed conversation loses what it was looking at.
func TestImageBlockComesBackOffDisk(t *testing.T) {
	message := Message{Role: RoleUser, Content: Content{
		TextBlock{Text: "why is this wrong"},
		ImageBlock{MediaType: "image/jpeg", Data: []byte{0xff, 0xd8, 0xff, 0x00}},
	}}

	raw, err := json.Marshal(message)
	if err != nil {
		t.Fatal(err)
	}
	var back Message
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatal(err)
	}

	if len(back.Content) != 2 {
		t.Fatalf("content = %+v", back.Content)
	}
	image, ok := back.Content[1].(ImageBlock)
	if !ok {
		t.Fatalf("content[1] = %T, want an ImageBlock", back.Content[1])
	}
	if image.MediaType != "image/jpeg" || string(image.Data) != "\xff\xd8\xff\x00" {
		t.Errorf("image = %+v", image)
	}
}
