package repl

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"github.com/zenodea/zaino/internal/agent"
	"github.com/zenodea/zaino/internal/config"
	"github.com/zenodea/zaino/internal/llm"
	"github.com/zenodea/zaino/internal/store/session"
)

func TestACommandFileBecomesAPrompt(t *testing.T) {
	o := Options{Config: &config.Config{Commands: []config.Command{
		{Name: "review", Description: "read the diff", Prompt: "Review $ARGUMENTS"},
	}}}

	var usage llm.Usage
	if got := runCommand(newAgent(), "/review the gate", nil, &usage, o); got.send != "Review the gate" {
		t.Errorf("send = %q", got.send)
	}
	if got := runCommand(newAgent(), "/nosuch", nil, &usage, o); got.send != "" {
		t.Errorf("an unknown command produced %q", got.send)
	}
}

func TestBroWaitsForAnAnswer(t *testing.T) {
	o := Options{Config: &config.Config{Commands: config.Builtins()}}
	var usage llm.Usage

	if got := runCommand(newAgent(), "/bro", nil, &usage, o); got.send != "" {
		t.Errorf("send = %q, want nothing said yet", got.send)
	}

	said := []llm.Message{llm.UserText("why"),
		{Role: llm.RoleAssistant, Content: llm.Content{llm.TextBlock{Text: "because"}}}}
	if got := runCommand(newAgent(), "/bro", said, &usage, o); !strings.Contains(got.send, "Re-explain") {
		t.Errorf("send = %q", got.send)
	}
}

func TestProfileAppliesAndComplains(t *testing.T) {
	ag := newAgent()
	o := Options{
		Recorder: session.NewRecorder(nil),
		Config: &config.Config{File: config.File{Profiles: map[string]config.Profile{
			"cheap": {Model: "stub-2", Effort: llm.EffortLow},
		}}},
	}

	if err := useProfile(ag, "cheap", o); err != nil {
		t.Fatal(err)
	}
	if ag.Model != "stub-2" || ag.Effort != llm.EffortLow {
		t.Errorf("model = %q, effort = %q", ag.Model, ag.Effort)
	}

	err := useProfile(ag, "cheep", o)
	if err == nil || !strings.Contains(err.Error(), "cheap") {
		t.Errorf("err = %v, want it to name what there is", err)
	}
}

func TestHelpListsWhatYouWrote(t *testing.T) {
	got := helpWith(&config.Config{Commands: append(config.Builtins(),
		config.Command{Name: "review", Description: "read the diff"})})

	if !strings.Contains(got, "/review") || !strings.Contains(got, "read the diff") {
		t.Errorf("help omits the command:\n%s", got)
	}
	if strings.Count(got, "/bro") != 1 {
		t.Errorf("/bro is listed twice:\n%s", got)
	}
}

// A summary that only reached the session file would leave the record and the
// conversation saying different things, and the next turn would re-send
// everything /compact was asked to fold away.
func TestCompactHandsTheFoldedContextBack(t *testing.T) {
	ag := newAgent()
	ag.Provider = &summariser{}
	ag.Compaction = &agent.Compaction{KeepRecent: 1}

	messages := []llm.Message{
		llm.UserText("first"),
		{Role: llm.RoleAssistant, Content: llm.Content{llm.TextBlock{Text: "an answer"}}},
		llm.UserText("second"),
	}

	var usage llm.Usage
	done := runCommand(ag, "/compact", messages, &usage, Options{Recorder: session.NewRecorder(nil)})
	if !done.changed {
		t.Fatal("/compact did not report a changed context")
	}
	if len(done.context) >= len(messages) {
		t.Fatalf("context = %d messages, want the older ones folded away", len(done.context))
	}
	if !strings.Contains(done.context[0].Text(), "what happened before") {
		t.Errorf("first message = %q, want the summary", done.context[0].Text())
	}
}

// A provider that answers anything with one line, which is all compaction
// needs of it.
type summariser struct{}

func (s *summariser) Name() string         { return "stub" }
func (s *summariser) DefaultModel() string { return "stub-1" }

func (s *summariser) Stream(context.Context, llm.Request) (llm.Stream, error) {
	return &oneLine{}, nil
}

type oneLine struct{}

func (o *oneLine) Next() bool       { return false }
func (o *oneLine) Event() llm.Event { return nil }
func (o *oneLine) Err() error       { return nil }
func (o *oneLine) Close() error     { return nil }

func (o *oneLine) Message() *llm.Response {
	return &llm.Response{Content: llm.Content{llm.TextBlock{Text: "what happened before"}}}
}

func TestRewindHandsBackAShorterConversation(t *testing.T) {
	store := &memStore{}
	o := Options{Recorder: session.NewRecorder(store)}

	messages := []llm.Message{
		llm.UserText("first thing"),
		{Role: llm.RoleAssistant, Content: llm.Content{llm.TextBlock{Text: "an answer"}}},
		llm.UserText("second thing"),
	}
	if err := o.Recorder.Messages(messages); err != nil {
		t.Fatal(err)
	}

	var usage llm.Usage
	done := runCommand(newAgent(), "/rewind 2", messages, &usage, o)
	if !done.changed {
		t.Fatal("the context was left as it was")
	}
	if len(done.context) != 2 {
		t.Errorf("context = %d messages, want the first exchange", len(done.context))
	}

	// Listing is not a change, and neither is a number that is not a turn.
	if got := runCommand(newAgent(), "/rewind", messages, &usage, o); got.changed {
		t.Error("listing the turns changed the context")
	}
	if got := runCommand(newAgent(), "/rewind 9", messages, &usage, o); got.changed {
		t.Error("a number past the end changed the context")
	}
}

// Enough of a store to record into; the file is not what is under test.
type memStore struct {
	entries []session.Entry
	leaf    string
}

func (s *memStore) Meta() session.Meta { return session.Meta{ID: "test"} }

func (s *memStore) Append(n session.New) (session.Entry, error) {
	e := session.Entry{ID: strconv.Itoa(len(s.entries) + 1), Parent: s.leaf, Seq: len(s.entries) + 1}
	raw, err := json.Marshal(n)
	if err != nil {
		return e, err
	}
	if err := json.Unmarshal(raw, &e); err != nil {
		return e, err
	}
	e.ID, e.Parent = strconv.Itoa(len(s.entries)+1), s.leaf
	s.entries = append(s.entries, e)
	s.leaf = e.ID
	return e, nil
}

func (s *memStore) Entries() ([]session.Entry, error) { return s.entries, nil }
func (s *memStore) Leaf() (string, error)             { return s.leaf, nil }
func (s *memStore) Close() error                      { return nil }

func (s *memStore) SetLeaf(id string) error {
	s.leaf = id
	return nil
}
