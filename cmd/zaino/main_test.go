package main

import (
	"path/filepath"
	"testing"
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
