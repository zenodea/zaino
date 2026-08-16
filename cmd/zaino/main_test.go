package main

import (
	"path/filepath"
	"testing"

	"github.com/zenodea/zaino/internal/agent"
	"github.com/zenodea/zaino/internal/llm"
	"github.com/zenodea/zaino/internal/store/session"
)

// Most runs have no mcp.json, so the empty session is the ordinary path and
// has to survive being appended from without a check.
func TestToolsSurviveHavingNoMCPServers(t *testing.T) {
	tests := []struct {
		name string
		off  bool
		path string
	}{
		{"switched off", true, ""},
		{"no config file", false, filepath.Join(t.TempDir(), "absent.json")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			servers, err := openMCP(tt.off, tt.path)
			if err != nil {
				t.Fatalf("openMCP: %v", err)
			}
			defer servers.Close()

			_, tools, err := openToolbox("manual", false, false, "", "")
			if err != nil {
				t.Fatalf("openToolbox: %v", err)
			}

			before := len(tools)
			tools = append(tools, servers.All()...)
			if len(tools) != before {
				t.Errorf("empty session contributed %d tools", len(tools)-before)
			}
			if before == 0 {
				t.Error("no tools at all")
			}
		})
	}
}

func TestCommaList(t *testing.T) {
	tests := []struct {
		in   string
		want int
	}{
		{"", 0},
		{"read", 1},
		{"read,grep", 2},
		{" read , grep ,, ", 2},
	}

	for _, tt := range tests {
		if got := commaList(tt.in); len(got) != tt.want {
			t.Errorf("commaList(%q) = %v, want %d entries", tt.in, got, tt.want)
		}
	}
}

func TestRestoredLimitIsAppliedUnlessTheFlagSaysOtherwise(t *testing.T) {
	limit := 200_000

	for _, tc := range []struct {
		name  string
		given map[string]bool
		start int
		want  int
	}{
		{name: "restored", given: map[string]bool{}, want: 200_000},
		{name: "flag wins", given: map[string]bool{"max-context": true}, start: 50_000, want: 50_000},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ag := &agent.Agent{Thinking: &llm.Thinking{}, MaxContext: tc.start}
			applyRestored(ag, session.Context{Limit: &limit}, tc.given)

			if ag.MaxContext != tc.want {
				t.Errorf("MaxContext = %d, want %d", ag.MaxContext, tc.want)
			}
		})
	}
}

func TestRestoredLimitOffIsNotMistakenForUnset(t *testing.T) {
	off := 0
	ag := &agent.Agent{Thinking: &llm.Thinking{}, MaxContext: 200_000}
	applyRestored(ag, session.Context{Limit: &off}, map[string]bool{})

	if ag.MaxContext != 0 {
		t.Errorf("MaxContext = %d, want the recorded ceiling of none", ag.MaxContext)
	}
}

func TestRecordSettingsWritesTheLimitOnlyWhenItChanges(t *testing.T) {
	limit := 200_000

	for _, tc := range []struct {
		name     string
		restored session.Context
		set      int
		want     bool
	}{
		{name: "new ceiling", set: 200_000, want: true},
		{name: "unchanged", restored: session.Context{Limit: &limit}, set: 200_000},
		{name: "turned off", restored: session.Context{Limit: &limit}, set: 0, want: true},
		{name: "never set", set: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo, err := session.OpenDir(t.TempDir(), "/work")
			if err != nil {
				t.Fatal(err)
			}
			store, err := repo.Create()
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()

			ag := &agent.Agent{Thinking: &llm.Thinking{}, MaxContext: tc.set}
			if err := recordSettings(session.NewRecorder(store), ag, "anthropic", tc.restored); err != nil {
				t.Fatal(err)
			}

			entries, err := store.Entries()
			if err != nil {
				t.Fatal(err)
			}
			written := false
			for _, e := range entries {
				if e.Type == session.KindLimit {
					written = true
					if e.Tokens == nil || *e.Tokens != tc.set {
						t.Errorf("recorded %v, want %d", e.Tokens, tc.set)
					}
				}
			}
			if written != tc.want {
				t.Errorf("limit written = %v, want %v", written, tc.want)
			}
		})
	}
}
