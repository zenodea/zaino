package tool

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/zenodea/zaino/internal/permission"
)

func workspace(t *testing.T) *Workspace {
	t.Helper()
	w, err := NewWorkspace(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return w
}

func write(t *testing.T, w *Workspace, name, content string) string {
	t.Helper()
	path := filepath.Join(w.Root, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func prepare(t *testing.T, tl Tool, args any) (Call, error) {
	t.Helper()
	raw, err := json.Marshal(args)
	if err != nil {
		t.Fatal(err)
	}
	return tl.Prepare(raw)
}

func run(t *testing.T, tl Tool, args any) (string, error) {
	t.Helper()
	call, err := prepare(t, tl, args)
	if err != nil {
		return "", err
	}
	return call.Run(context.Background())
}

func mustRun(t *testing.T, tl Tool, args any) string {
	t.Helper()
	out, err := run(t, tl, args)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	return out
}

func TestResolveStaysInsideWorkspace(t *testing.T) {
	w := workspace(t)

	tests := []struct {
		name        string
		path        string
		wantOutside bool
	}{
		{"plain", "main.go", false},
		{"nested", "internal/llm/types.go", false},
		{"dot", "./main.go", false},
		{"up and back", "internal/../main.go", false},
		{"parent", "../secrets", true},
		{"deep parent", "../../../etc/passwd", true},
		{"absolute outside", "/etc/hosts", true},
		{"absolute inside", filepath.Join(w.Root, "main.go"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := w.Resolve(tt.path)
			if err != nil {
				t.Fatalf("Resolve(%q): %v", tt.path, err)
			}
			if got.Outside != tt.wantOutside {
				t.Errorf("Resolve(%q).Outside = %v, want %v (abs %s)", tt.path, got.Outside, tt.wantOutside, got.Abs)
			}
		})
	}
}

func TestResolveFollowsSymlinksBeforeJudging(t *testing.T) {
	w := workspace(t)
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(w.Root, "escape")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	got, err := w.Resolve("escape/secrets.txt")
	if err != nil {
		t.Fatal(err)
	}
	if !got.Outside {
		t.Errorf("Resolve through a symlink = inside (%s), want outside", got.Abs)
	}
}

func TestResolveRejectsEmpty(t *testing.T) {
	if _, err := workspace(t).Resolve("  "); err == nil {
		t.Error("Resolve(\"\") = nil error, want a complaint")
	}
}

func TestReadNumbersLines(t *testing.T) {
	w := workspace(t)
	write(t, w, "a.txt", "one\ntwo\nthree\n")

	out := mustRun(t, &Read{w}, readArgs{Path: "a.txt"})
	want := "     1\tone\n     2\ttwo\n     3\tthree\n"
	if out != want {
		t.Errorf("read =\n%q\nwant\n%q", out, want)
	}
}

func TestReadPaging(t *testing.T) {
	w := workspace(t)
	write(t, w, "a.txt", "one\ntwo\nthree\nfour\n")

	out := mustRun(t, &Read{w}, readArgs{Path: "a.txt", Offset: 2, Limit: 2})
	if !strings.Contains(out, "     2\ttwo") || !strings.Contains(out, "     3\tthree") {
		t.Errorf("read = %q, want lines 2 and 3", out)
	}
	if strings.Contains(out, "one") || strings.Contains(out, "four") {
		t.Errorf("read = %q, want only the requested window", out)
	}
	if !strings.Contains(out, "1 more lines") {
		t.Errorf("read = %q, want a note that more remains", out)
	}
}

func TestReadRejects(t *testing.T) {
	w := workspace(t)
	write(t, w, "binary", "text\x00more")
	if err := os.Mkdir(filepath.Join(w.Root, "dir"), 0o755); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		path string
		want string
	}{
		{"missing", "nope.txt", "does not exist"},
		{"binary", "binary", "binary file"},
		{"directory", "dir", "is a directory"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := run(t, &Read{w}, readArgs{Path: tt.path})
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("read err = %v, want one mentioning %q", err, tt.want)
			}
		})
	}
}

func TestReadIsWhatUnlocksEditing(t *testing.T) {
	w := workspace(t)
	write(t, w, "a.txt", "hello\n")

	args := editArgs{Path: "a.txt", OldText: "hello", NewText: "goodbye"}
	if _, err := prepare(t, &Edit{w}, args); err == nil || !strings.Contains(err.Error(), "read a.txt before") {
		t.Fatalf("edit before read = %v, want a refusal", err)
	}

	mustRun(t, &Read{w}, readArgs{Path: "a.txt"})
	if _, err := run(t, &Edit{w}, args); err != nil {
		t.Fatalf("edit after read: %v", err)
	}
	if got := read(t, w, "a.txt"); got != "goodbye\n" {
		t.Errorf("file = %q, want %q", got, "goodbye\n")
	}
}

func read(t *testing.T, w *Workspace, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(w.Root, name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestEditAmbiguity(t *testing.T) {
	w := workspace(t)
	write(t, w, "a.txt", "x\nx\n")
	mustRun(t, &Read{w}, readArgs{Path: "a.txt"})

	_, err := prepare(t, &Edit{w}, editArgs{Path: "a.txt", OldText: "x", NewText: "y"})
	if err == nil || !strings.Contains(err.Error(), "appears 2 times") {
		t.Fatalf("ambiguous edit = %v, want a refusal naming the count", err)
	}

	if _, err := run(t, &Edit{w}, editArgs{Path: "a.txt", OldText: "x", NewText: "y", ReplaceAll: true}); err != nil {
		t.Fatalf("replace_all: %v", err)
	}
	if got := read(t, w, "a.txt"); got != "y\ny\n" {
		t.Errorf("file = %q, want %q", got, "y\ny\n")
	}
}

func TestEditForgivesTypography(t *testing.T) {
	w := workspace(t)
	write(t, w, "a.go", "s := “hello” // an em—dash\n")
	mustRun(t, &Read{w}, readArgs{Path: "a.go"})

	out, err := run(t, &Edit{w}, editArgs{
		Path:    "a.go",
		OldText: `s := "hello" // an em-dash`,
		NewText: `s := "goodbye"`,
	})
	if err != nil {
		t.Fatalf("edit: %v", err)
	}
	if !strings.Contains(out, "normalising quotes and dashes") {
		t.Errorf("edit = %q, want it to say the match was fuzzy", out)
	}
	if got := read(t, w, "a.go"); got != "s := \"goodbye\"\n" {
		t.Errorf("file = %q", got)
	}
}

func TestEditForgivesTrailingWhitespace(t *testing.T) {
	w := workspace(t)
	write(t, w, "a.go", "func main() {   \n\tprintln(1)\t\n}\n")
	mustRun(t, &Read{w}, readArgs{Path: "a.go"})

	out, err := run(t, &Edit{w}, editArgs{
		Path:    "a.go",
		OldText: "func main() {\n\tprintln(1)\n}",
		NewText: "func main() {\n\tprintln(2)\n}",
	})
	if err != nil {
		t.Fatalf("edit: %v", err)
	}
	if !strings.Contains(out, "ignoring trailing whitespace") {
		t.Errorf("edit = %q, want it to say the match was fuzzy", out)
	}
	if got := read(t, w, "a.go"); !strings.Contains(got, "println(2)") {
		t.Errorf("file = %q", got)
	}
}

func TestEditRefusesAStaleFile(t *testing.T) {
	w := workspace(t)
	write(t, w, "a.txt", "hello\n")
	mustRun(t, &Read{w}, readArgs{Path: "a.txt"})

	call, err := prepare(t, &Edit{w}, editArgs{Path: "a.txt", OldText: "hello", NewText: "goodbye"})
	if err != nil {
		t.Fatal(err)
	}
	write(t, w, "a.txt", "someone else was here\n")

	if _, err := call.Run(context.Background()); err == nil || !strings.Contains(err.Error(), "changed on disk") {
		t.Fatalf("stale edit = %v, want a refusal", err)
	}
	if got := read(t, w, "a.txt"); got != "someone else was here\n" {
		t.Errorf("file = %q, want the other writer's content untouched", got)
	}
}

func TestEditRejects(t *testing.T) {
	w := workspace(t)
	write(t, w, "a.txt", "hello\n")
	mustRun(t, &Read{w}, readArgs{Path: "a.txt"})

	tests := []struct {
		name string
		args editArgs
		want string
	}{
		{"no old_text", editArgs{Path: "a.txt", NewText: "x"}, "old_text is required"},
		{"identical", editArgs{Path: "a.txt", OldText: "hello", NewText: "hello"}, "identical"},
		{"absent", editArgs{Path: "a.txt", OldText: "nowhere", NewText: "x"}, "does not appear"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := prepare(t, &Edit{w}, tt.args)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("edit err = %v, want one mentioning %q", err, tt.want)
			}
		})
	}
}

func TestEditPreviewIsADiff(t *testing.T) {
	w := workspace(t)
	write(t, w, "a.txt", "one\ntwo\nthree\n")
	mustRun(t, &Read{w}, readArgs{Path: "a.txt"})

	call, err := prepare(t, &Edit{w}, editArgs{Path: "a.txt", OldText: "two", NewText: "TWO"})
	if err != nil {
		t.Fatal(err)
	}
	req := call.Request()
	if req.Action != permission.Write {
		t.Errorf("Action = %q, want write", req.Action)
	}
	if !strings.Contains(req.Preview, "- two") || !strings.Contains(req.Preview, "+ TWO") {
		t.Errorf("Preview =\n%s\nwant both sides of the change", req.Preview)
	}
	if strings.Contains(req.Preview, "one") || strings.Contains(req.Preview, "three") {
		t.Errorf("Preview =\n%s\nwant only the changed lines", req.Preview)
	}
}

func TestWriteCreates(t *testing.T) {
	w := workspace(t)

	out := mustRun(t, &Write{w}, writeArgs{Path: "new/deep.txt", Content: "hi\n"})
	if !strings.Contains(out, "Created") {
		t.Errorf("write = %q", out)
	}
	if got := read(t, w, "new/deep.txt"); got != "hi\n" {
		t.Errorf("file = %q", got)
	}
}

func TestWriteWontClobberUnread(t *testing.T) {
	w := workspace(t)
	write(t, w, "a.txt", "precious\n")

	_, err := prepare(t, &Write{w}, writeArgs{Path: "a.txt", Content: "gone\n"})
	if err == nil || !strings.Contains(err.Error(), "read it before overwriting") {
		t.Fatalf("write over unread = %v, want a refusal", err)
	}
	if got := read(t, w, "a.txt"); got != "precious\n" {
		t.Errorf("file = %q, want it untouched", got)
	}
}

func TestLs(t *testing.T) {
	w := workspace(t)
	write(t, w, "b.txt", "")
	write(t, w, "a.txt", "")
	write(t, w, ".hidden", "")
	if err := os.Mkdir(filepath.Join(w.Root, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}

	out := mustRun(t, &Ls{w}, lsArgs{})
	if out != "a.txt\nb.txt\nsub/" {
		t.Errorf("ls = %q, want sorted with a trailing slash on the directory", out)
	}

	if all := mustRun(t, &Ls{w}, lsArgs{All: true}); !strings.Contains(all, ".hidden") {
		t.Errorf("ls all = %q, want the dotfile", all)
	}
}

func TestFind(t *testing.T) {
	w := workspace(t)
	write(t, w, "main.go", "")
	write(t, w, "internal/llm/types.go", "")
	write(t, w, "internal/llm/types_test.go", "")
	write(t, w, "README.md", "")
	write(t, w, "node_modules/dep/index.go", "")

	tests := []struct {
		pattern string
		want    []string
		absent  []string
	}{
		{"*.go", []string{"main.go", "internal/llm/types.go"}, []string{"README.md"}},
		{"*_test.go", []string{"internal/llm/types_test.go"}, []string{"internal/llm/types.go"}},
		{"internal/**/*.go", []string{"internal/llm/types.go"}, []string{"main.go"}},
		{"*.md", []string{"README.md"}, []string{"main.go"}},
	}

	for _, tt := range tests {
		t.Run(tt.pattern, func(t *testing.T) {
			out := mustRun(t, &Find{w}, findArgs{Pattern: tt.pattern})
			for _, want := range tt.want {
				if !strings.Contains(out, want) {
					t.Errorf("find %q =\n%s\nwant it to include %s", tt.pattern, out, want)
				}
			}
			for _, absent := range tt.absent {
				if strings.Contains(out, absent) {
					t.Errorf("find %q =\n%s\nwant it to leave out %s", tt.pattern, out, absent)
				}
			}
			if strings.Contains(out, "node_modules") {
				t.Errorf("find %q =\n%s\nwant node_modules skipped", tt.pattern, out)
			}
		})
	}
}

func TestMatchGlob(t *testing.T) {
	tests := []struct {
		pattern, name string
		want          bool
	}{
		{"*.go", "main.go", true},
		{"*.go", "sub/main.go", false},
		{"**/*.go", "sub/main.go", true},
		{"**/*.go", "main.go", true},
		{"**/*.go", "a/b/c/main.go", true},
		{"internal/**/*.go", "internal/llm/types.go", true},
		{"internal/**/*.go", "cmd/main.go", false},
		{"internal/*", "internal/llm", true},
		{"internal/*", "internal/llm/types.go", false},
	}

	for _, tt := range tests {
		t.Run(tt.pattern+" vs "+tt.name, func(t *testing.T) {
			if got := matchGlob(tt.pattern, tt.name); got != tt.want {
				t.Errorf("matchGlob(%q, %q) = %v, want %v", tt.pattern, tt.name, got, tt.want)
			}
		})
	}
}

func TestGrep(t *testing.T) {
	w := workspace(t)
	write(t, w, "a.go", "package main\nfunc main() {}\n")
	write(t, w, "b.txt", "func in a text file\n")

	out := mustRun(t, &Grep{w}, grepArgs{Pattern: `func \w+\(`})
	if !strings.Contains(out, "a.go:2:func main() {}") {
		t.Errorf("grep =\n%s\nwant the Go match with path and line", out)
	}

	filtered := mustRun(t, &Grep{w}, grepArgs{Pattern: "func", Glob: "*.go"})
	if strings.Contains(filtered, "b.txt") {
		t.Errorf("grep with glob =\n%s\nwant the text file filtered out", filtered)
	}

	none := mustRun(t, &Grep{w}, grepArgs{Pattern: "absolutely-not-here"})
	if !strings.Contains(none, "no matches") {
		t.Errorf("grep = %q, want a clear miss", none)
	}
}

func TestGrepRejectsBadPattern(t *testing.T) {
	if _, err := prepare(t, &Grep{workspace(t)}, grepArgs{Pattern: "("}); err == nil {
		t.Error("grep with an unparseable pattern = nil error")
	}
}

func TestBash(t *testing.T) {
	w := workspace(t)
	write(t, w, "marker.txt", "")

	if out := mustRun(t, &Bash{w}, bashArgs{Command: "ls"}); !strings.Contains(out, "marker.txt") {
		t.Errorf("bash ls = %q, want it to run in the workspace", out)
	}
	if out := mustRun(t, &Bash{w}, bashArgs{Command: "true"}); out != "(no output)" {
		t.Errorf("bash = %q, want a note that there was no output", out)
	}

	_, err := run(t, &Bash{w}, bashArgs{Command: "echo oops >&2; exit 3"})
	if err == nil || !strings.Contains(err.Error(), "exit status 3") {
		t.Errorf("bash err = %v, want the exit status", err)
	}
	if err == nil || !strings.Contains(err.Error(), "oops") {
		t.Errorf("bash err = %v, want stderr carried back", err)
	}
}

func TestBashAsksToExecute(t *testing.T) {
	call, err := prepare(t, &Bash{workspace(t)}, bashArgs{Command: "git push"})
	if err != nil {
		t.Fatal(err)
	}
	req := call.Request()
	if req.Action != permission.Execute {
		t.Errorf("Action = %q, want execute", req.Action)
	}
	if req.Target != "git push" {
		t.Errorf("Target = %q, want the command", req.Target)
	}
}

func TestReadOnlyToolsAskForRead(t *testing.T) {
	w := workspace(t)
	write(t, w, "a.txt", "x")

	for _, tl := range []Tool{&Read{w}, &Ls{w}, &Find{w}, &Grep{w}} {
		name := tl.Definition().Name
		t.Run(name, func(t *testing.T) {
			call, err := prepare(t, tl, map[string]any{"path": "a.txt", "pattern": "x"})
			if err != nil {
				t.Fatal(err)
			}
			if got := call.Request().Action; got != permission.Read {
				t.Errorf("%s action = %q, want read", name, got)
			}
		})
	}
}

func TestSelect(t *testing.T) {
	all := All(workspace(t))

	only, err := Select(all, []string{"read", "grep"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := Names(only); strings.Join(got, ",") != "read,grep" {
		t.Errorf("Select(allow) = %v", got)
	}

	without, err := Select(all, nil, []string{"bash"})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range Names(without) {
		if name == "bash" {
			t.Error("Select(deny) kept bash")
		}
	}
	if len(without) != len(all)-1 {
		t.Errorf("Select(deny) kept %d tools, want %d", len(without), len(all)-1)
	}

	if _, err := Select(all, []string{"nonesuch"}, nil); err == nil {
		t.Error("Select with an unknown name = nil error")
	}
}

func TestAllToolsHaveSchemas(t *testing.T) {
	for _, tl := range All(workspace(t)) {
		def := tl.Definition()
		if def.Name == "" || def.Description == "" {
			t.Errorf("%+v is missing a name or description", def)
		}
		if def.InputSchema["type"] != "object" {
			t.Errorf("%s schema = %v, want an object", def.Name, def.InputSchema)
		}
		if _, err := json.Marshal(def.InputSchema); err != nil {
			t.Errorf("%s schema does not marshal: %v", def.Name, err)
		}
	}
}

func TestEditAppliesSeveralEditsInOneCall(t *testing.T) {
	w := workspace(t)
	write(t, w, "a.go", "one\ntwo\nthree\n")
	mustRun(t, &Read{w}, readArgs{Path: "a.go"})

	out, err := run(t, &Edit{w}, editArgs{Path: "a.go", Edits: []editStep{
		{OldText: "one", NewText: "ONE"},
		{OldText: "three", NewText: "THREE"},
	}})
	if err != nil {
		t.Fatalf("edit: %v", err)
	}
	if got := read(t, w, "a.go"); got != "ONE\ntwo\nTHREE\n" {
		t.Errorf("file = %q", got)
	}
	if !strings.Contains(out, "2 edits") {
		t.Errorf("out = %q, want it to say how many edits landed", out)
	}
}

// Each step sees the result of the last, so a later edit can touch what an
// earlier one wrote.
func TestEditsSeeEachOther(t *testing.T) {
	w := workspace(t)
	write(t, w, "a.go", "alpha\n")
	mustRun(t, &Read{w}, readArgs{Path: "a.go"})

	if _, err := run(t, &Edit{w}, editArgs{Path: "a.go", Edits: []editStep{
		{OldText: "alpha", NewText: "beta"},
		{OldText: "beta", NewText: "gamma"},
	}}); err != nil {
		t.Fatalf("edit: %v", err)
	}
	if got := read(t, w, "a.go"); got != "gamma\n" {
		t.Errorf("file = %q, want the second edit to have seen the first", got)
	}
}

// A batch that failed part way through would leave the file in a state nobody
// asked for, so nothing is written unless everything matches.
func TestAFailedEditInABatchWritesNothing(t *testing.T) {
	w := workspace(t)
	write(t, w, "a.go", "one\ntwo\n")
	mustRun(t, &Read{w}, readArgs{Path: "a.go"})

	_, err := run(t, &Edit{w}, editArgs{Path: "a.go", Edits: []editStep{
		{OldText: "one", NewText: "ONE"},
		{OldText: "nowhere", NewText: "x"},
	}})
	if err == nil || !strings.Contains(err.Error(), "edit 2 of 2") {
		t.Fatalf("err = %v, want it to name the step that failed", err)
	}
	if got := read(t, w, "a.go"); got != "one\ntwo\n" {
		t.Errorf("file = %q, want it untouched", got)
	}
}

func TestSingleEditStillWorks(t *testing.T) {
	w := workspace(t)
	write(t, w, "a.go", "hello\n")
	mustRun(t, &Read{w}, readArgs{Path: "a.go"})

	if _, err := run(t, &Edit{w}, editArgs{Path: "a.go", OldText: "hello", NewText: "goodbye"}); err != nil {
		t.Fatalf("edit: %v", err)
	}
	if got := read(t, w, "a.go"); got != "goodbye\n" {
		t.Errorf("file = %q", got)
	}
}

func TestBatchAsksOnceWithOneDiff(t *testing.T) {
	w := workspace(t)
	write(t, w, "a.go", "one\ntwo\nthree\n")
	mustRun(t, &Read{w}, readArgs{Path: "a.go"})

	call, err := prepare(t, &Edit{w}, editArgs{Path: "a.go", Edits: []editStep{
		{OldText: "one", NewText: "ONE"},
		{OldText: "three", NewText: "THREE"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	req := call.Request()
	if !strings.Contains(req.Preview, "ONE") || !strings.Contains(req.Preview, "THREE") {
		t.Errorf("preview shows only part of the batch:\n%s", req.Preview)
	}
}

func TestBashStreamsOutputToWhoeverListens(t *testing.T) {
	w := workspace(t)
	call, err := prepare(t, &Bash{w}, bashArgs{Command: "echo one; sleep 0.05; echo two; echo three >&2"})
	if err != nil {
		t.Fatal(err)
	}

	var mu sync.Mutex
	var chunks []string
	ctx := WithProgress(context.Background(), func(chunk string) {
		mu.Lock()
		defer mu.Unlock()
		chunks = append(chunks, chunk)
	})

	out, err := call.Run(ctx)
	if err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(chunks) < 2 {
		t.Errorf("got %d chunks, want output to arrive as it happens: %q", len(chunks), chunks)
	}
	if joined := strings.Join(chunks, ""); strings.TrimRight(joined, "\n") != out {
		t.Errorf("streamed %q but returned %q", joined, out)
	}
	for _, want := range []string{"one", "two", "three"} {
		if !strings.Contains(out, want) {
			t.Errorf("output %q is missing %q", out, want)
		}
	}
}
