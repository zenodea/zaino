package tool

import (
	"strings"
	"testing"
)

func TestTodoReplacesTheListAndCounts(t *testing.T) {
	td := NewTodo()
	if got := td.Render(); !strings.Contains(got, "no plan") {
		t.Errorf("empty = %q", got)
	}

	out, err := runTool(t, td, `{"items": [{"text": "read the code"}, {"text": "change it", "status": "active"}, {"text": "test", "status": "Done"}]}`)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"[ ] read the code", "[>] change it", "[x] test", "1 of 3 done"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in %q", want, out)
		}
	}
	if done, total := td.Progress(); done != 1 || total != 3 {
		t.Errorf("progress = %d/%d", done, total)
	}

	if _, err := runTool(t, td, `{"items": [{"text": "just this"}]}`); err != nil {
		t.Fatal(err)
	}
	if items := td.Items(); len(items) != 1 || items[0].Status != "pending" {
		t.Errorf("after replace = %+v", items)
	}
}

func TestTodoRefusesWhatIsNotAStep(t *testing.T) {
	td := NewTodo()
	for _, input := range []string{`{"items": [{"text": ""}]}`, `{"items": [{"text": "x", "status": "maybe"}]}`} {
		if _, err := td.Prepare([]byte(input)); err == nil {
			t.Errorf("%s: want an error", input)
		}
	}
}
