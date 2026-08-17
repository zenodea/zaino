package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zenodea/zaino/internal/config"
)

func settingsFor(model, effort *string, thinking *bool) knobs {
	return knobs{
		provider: new(string), model: model, maxTokens: new(int), effort: effort,
		system: new(string), thinking: thinking, permission: new(string),
		allowOutside: new(bool), tools: new(string), excludeTools: new(string),
		contextWindow: new(int), maxContext: new(string), vim: new(bool),
		mouse: new(bool), animate: new(bool),
	}
}

func TestConfigFillsInWhatTheFlagsDidNot(t *testing.T) {
	model, effort := new(string), new(string)
	*effort = "high" // as typed on the command line

	cfg := &config.Config{File: config.File{Model: "from-config", Effort: "low"}}
	if err := settingsFor(model, effort, new(bool)).apply(cfg, "", map[string]bool{"effort": true}); err != nil {
		t.Fatal(err)
	}

	if *model != "from-config" {
		t.Errorf("model = %q, want the config's", *model)
	}
	if *effort != "high" {
		t.Errorf("effort = %q, want the flag to win", *effort)
	}
}

func TestProfileBeatsThePlainKeys(t *testing.T) {
	model := new(string)
	yes := true
	cfg := &config.Config{File: config.File{
		Model:    "everyday",
		Profiles: map[string]config.Profile{"deep": {Model: "the-big-one", Thinking: &yes}},
	}}

	thinking := new(bool)
	if err := settingsFor(model, new(string), thinking).apply(cfg, "deep", nil); err != nil {
		t.Fatal(err)
	}
	if *model != "the-big-one" {
		t.Errorf("model = %q, want the profile's", *model)
	}
	if !*thinking {
		t.Error("the profile did not turn thinking on")
	}
}

// -continue restores the session's settings over anything a config file said,
// but a profile asked for by name is as deliberate as a flag.
func TestAskedForProfileCountsAsTyped(t *testing.T) {
	cfg := &config.Config{File: config.File{
		Profile:  "quiet",
		Profiles: map[string]config.Profile{"quiet": {Model: "small"}, "loud": {Effort: "max"}},
	}}

	given := map[string]bool{"profile": true}
	if err := settingsFor(new(string), new(string), new(bool)).apply(cfg, "loud", given); err != nil {
		t.Fatal(err)
	}
	if !given["effort"] {
		t.Error("the profile's effort will be overwritten by a resumed session")
	}
	if given["model"] {
		t.Error("a profile stamped a setting it does not touch")
	}

	// The one named in the config is a default, and defers to the session.
	given = map[string]bool{}
	if err := settingsFor(new(string), new(string), new(bool)).apply(cfg, "", given); err != nil {
		t.Fatal(err)
	}
	if given["model"] {
		t.Error("a profile from the config file counted as typed")
	}
}

func TestUnknownProfileSaysWhatThereIs(t *testing.T) {
	cfg := &config.Config{File: config.File{Profiles: map[string]config.Profile{"deep": {}}}}
	err := settingsFor(new(string), new(string), new(bool)).apply(cfg, "depe", nil)
	if err == nil {
		t.Fatal("got nil, want an error")
	}
	if !strings.Contains(err.Error(), "deep") {
		t.Errorf("error does not name the profiles there are: %v", err)
	}
}

func TestReadPrompt(t *testing.T) {
	if got, err := readPrompt("be terse"); err != nil || got != "be terse" {
		t.Errorf("got %q, %v — a plain prompt is itself", got, err)
	}

	path := filepath.Join(t.TempDir(), "system.md")
	if err := os.WriteFile(path, []byte("  from a file\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, err := readPrompt("@" + path); err != nil || got != "from a file" {
		t.Errorf("got %q, %v", got, err)
	}
	if _, err := readPrompt("@" + filepath.Join(t.TempDir(), "absent")); err == nil {
		t.Error("a missing file went unreported")
	}
}
