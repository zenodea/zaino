package tui

import (
	"strings"
	"testing"

	"github.com/zenodea/zaino/internal/llm"
	"github.com/zenodea/zaino/internal/store/session"
)

// A conversation of two exchanges, rewound to the second turn and asked
// differently, so the session file holds a fork.
func newBranchedModel(t *testing.T) *Model {
	t.Helper()
	m := newTestModel(t, 80, 24)
	repo, rec := openStore(t)
	m.UseSession(repo, rec)

	said(m, "first thing", "an answer", "second thing", "another answer")
	if err := m.rec.Messages(m.messages); err != nil {
		t.Fatal(err)
	}
	if err := m.rec.Rewind(2); err != nil {
		t.Fatal(err)
	}
	m.messages = m.messages[:2]
	said(m, "a different second", "a different answer")
	if err := m.rec.Messages(m.messages); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestJourneyShowsTheBranchLeftBehind(t *testing.T) {
	m := newBranchedModel(t)

	m.runCommand("/journey")
	if !m.journey.open {
		t.Fatal("the journey view did not open")
	}

	stops := m.journey.stops
	if len(stops) != 3 {
		t.Fatalf("%d stops, want all three turns: %+v", len(stops), stops)
	}
	if stops[0].prompt != "first thing" || stops[1].prompt != "second thing" ||
		stops[2].prompt != "a different second" {
		t.Errorf("stops = %q, %q, %q", stops[0].prompt, stops[1].prompt, stops[2].prompt)
	}
	if !stops[0].onPath || stops[1].onPath || !stops[2].onPath {
		t.Errorf("path marks = %v %v %v, want the abandoned turn unlit",
			stops[0].onPath, stops[1].onPath, stops[2].onPath)
	}
	if !stops[2].here || stops[0].here || stops[1].here {
		t.Errorf("here should be the newest turn on the path")
	}
	if stops[1].messages != 4 {
		t.Errorf("signpost says landing on the abandoned turn holds %d messages, want its whole exchange, 4",
			stops[1].messages)
	}
	if m.journey.branches != 2 {
		t.Errorf("branches = %d, want the two ways the tree ends", m.journey.branches)
	}
	if m.journey.cursor != 2 {
		t.Errorf("cursor = %d, want it starting where you are", m.journey.cursor)
	}
}

func TestJourneyBranchesFromAnAbandonedTurn(t *testing.T) {
	m := newBranchedModel(t)

	m.runCommand("/journey")
	m.Update(key("up"))
	m.Update(key("b"))

	if m.journey.open {
		t.Fatal("the journey view stayed open")
	}
	if len(m.messages) != 2 {
		t.Fatalf("messages = %d, want the first exchange alone", len(m.messages))
	}
	if m.input.Value() != "second thing" {
		t.Errorf("composer = %q, want the abandoned prompt back", m.input.Value())
	}
	if last := m.entries[len(m.entries)-1]; !strings.Contains(last.text, "branched") {
		t.Errorf("entry = %+v", last)
	}
}

func TestJourneyVisitsATurnAskedAndAnswered(t *testing.T) {
	m := newBranchedModel(t)

	m.runCommand("/journey")
	m.Update(key("up"))
	m.Update(key("enter"))

	if m.journey.open {
		t.Fatal("the journey view stayed open")
	}
	if len(m.messages) != 4 {
		t.Fatalf("messages = %d, want the abandoned road's two whole exchanges", len(m.messages))
	}
	if got := m.messages[3].Text(); got != "another answer" {
		t.Errorf("last message = %q, want the answer already there", got)
	}
	if m.input.Value() != "" {
		t.Errorf("composer = %q, want it left alone", m.input.Value())
	}
	if last := m.entries[len(m.entries)-1]; !strings.Contains(last.text, "travelled") {
		t.Errorf("entry = %+v", last)
	}
}

// Whatever is recorded after a jump has to hang off the chosen turn, not off
// the branch that was on screen before it.
func TestAJumpMovesWhereRecordingHangs(t *testing.T) {
	m := newBranchedModel(t)

	m.runCommand("/journey")
	m.Update(key("up"))
	m.Update(key("enter"))

	said(m, "a third second", "a third answer")
	if err := m.rec.Messages(m.messages); err != nil {
		t.Fatal(err)
	}

	m.runCommand("/journey")
	stops := m.journey.stops
	if len(stops) != 4 {
		t.Fatalf("%d stops, want a third turn grown: %+v", len(stops), stops)
	}
	for _, stop := range stops {
		if stop.prompt == "a third second" {
			if !stop.here {
				t.Errorf("the turn just asked is not where recording hangs: %+v", stop)
			}
			return
		}
	}
	t.Errorf("the turn just asked is not on the map: %+v", stops)
}

func TestJourneyWithoutAStore(t *testing.T) {
	m := newTestModel(t, 80, 24)

	m.runCommand("/journey")
	if m.journey.open {
		t.Fatal("opened with nothing to show")
	}
	if last := m.entries[len(m.entries)-1]; !strings.Contains(last.text, "no session on disk") {
		t.Errorf("entry = %+v", last)
	}
}

func TestAbandonedBranchesCarryNoWarmInk(t *testing.T) {
	m := newBranchedModel(t)
	m.runCommand("/journey")

	dashed := false
	for _, line := range m.journey.lines {
		if strings.ContainsAny(line.road, "┡┠") {
			t.Fatalf("road %q draws both roads in one glyph", line.road)
		}
		dashed = dashed || strings.Contains(line.road, "┃╌")
	}
	if !dashed {
		t.Error("no branch leaves the trunk as a plain dash")
	}
}

func TestJourneyShowsTheContextAtEachStop(t *testing.T) {
	m := newTestModel(t, 80, 24)
	repo, rec := openStore(t)
	m.UseSession(repo, rec)

	said(m, "first thing", "an answer")
	m.rec.Turn(llm.Usage{InputTokens: 190_000, OutputTokens: 10_000})
	if err := m.rec.Messages(m.messages); err != nil {
		t.Fatal(err)
	}

	m.runCommand("/journey")
	if got := m.journey.stops[0].tokens; got != 200_000 {
		t.Fatalf("tokens = %d, want what the provider counted", got)
	}
	if view := m.journeyView(); !strings.Contains(view, "200.0k") {
		t.Errorf("view does not carry the context size:\n%s", view)
	}
}

// A model change made after a turn, on one of two roads out of it, is drawn
// past that road's bend and not on the trunk both roads share — and a row
// down from the corner, so it never sits on the bend itself.
func TestALandmarkPastAForkStaysOnItsRoad(t *testing.T) {
	user := func(id, parent, text string) session.Entry {
		e := session.Entry{ID: id, Parent: parent, Type: session.KindMessage}
		msg := llm.UserText(text)
		e.Message = &msg
		return e
	}
	reply := func(id, parent, text string) session.Entry {
		e := session.Entry{ID: id, Parent: parent, Type: session.KindMessage}
		msg := llm.Message{Role: llm.RoleAssistant, Content: llm.Content{llm.TextBlock{Text: text}}}
		e.Message = &msg
		return e
	}
	model := session.Entry{ID: "m", Parent: "p2", Type: session.KindModel}
	model.Model = "@preset/cheap"

	entries := []session.Entry{
		user("p1", "", "do it"), reply("p2", "p1", "done"),
		model,
		user("s1", "m", "spawn"), reply("s2", "s1", "spawned"),
		user("d1", "p2", "do it again"), reply("d2", "d1", "again"),
	}
	lines, _, _ := draw(entries, session.PathTo(entries, "s2"))

	var drawn []string
	for _, l := range lines {
		drawn = append(drawn, l.road+l.text)
	}
	got := strings.Join(drawn, "\n")
	want := strings.Join([]string{
		"",
		"┃",
		"┗━┓",
		"┆ ┃",
		"┆ ⇄ model → @preset/cheap",
		"┆ ┃",
		"┆ ",
		"┆",
		"╰╌",
	}, "\n")
	if got != want {
		t.Errorf("map drawn as\n%s\nwant\n%s", got, want)
	}
}
