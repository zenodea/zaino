package tui

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/zenodea/zaino/internal/store/session"
)

const restoreListed = 12

func (m *Model) offerRestore(entries []session.Entry, from, to string) {
	if m.blobs == nil {
		return
	}
	plan := session.PlanFiles(entries, from, to)
	if len(plan) == 0 {
		return
	}
	conflicts := session.Conflicts(m.root, plan)

	var listed []string
	for i, s := range plan {
		if i == restoreListed {
			listed = append(listed, hintStyle.Render(fmt.Sprintf("… and %d more", len(plan)-i)))
			break
		}
		what := "put back"
		switch {
		case !s.Restorable:
			what = "too big to have kept"
		case !s.WantExists:
			what = "remove"
		}
		listed = append(listed, keyed(s.Path, what))
	}

	restore := choice{label: "put the files back", value: "yes",
		detail: fmt.Sprintf("%d files as they were there", len(plan)), body: listed}
	keep := choice{label: "leave the files as they are", value: "no",
		detail: "the conversation moves, the files stay", body: listed}
	options := []choice{restore, keep}
	if len(conflicts) > 0 {
		restore.detail += fmt.Sprintf(", skipping %d changed outside zaino", len(conflicts))
		options[0] = restore
		var changed []string
		for _, c := range conflicts {
			changed = append(changed, keyed(c, "changed outside zaino since"))
		}
		options = append(options, choice{label: "put them back, overwriting", value: "force",
			detail: fmt.Sprintf("%d changed outside zaino go too", len(conflicts)), body: changed})
	}

	m.runCmd(m.ask(chooser{title: "files · put them back too?", options: options,
		apply: func(m *Model, picked choice) {
			if picked.value == "no" {
				m.notice("files left as they are · /files shows what this line changed")
				return
			}
			m.restore(plan, picked.value == "force")
		}}))
}

func (m *Model) restore(plan []session.FileState, force bool) {
	done, err := session.RestoreFiles(m.root, m.blobs, plan, force)
	if err != nil {
		m.push(entry{kind: entryError, text: "restoring files: " + err.Error()})
	}
	parts := []string{fmt.Sprintf("%d files put back", len(done.Files))}
	if len(done.Conflicts) > 0 {
		parts = append(parts, fmt.Sprintf("%d left alone, changed outside zaino", len(done.Conflicts)))
	}
	if len(done.Skipped) > 0 {
		parts = append(parts, fmt.Sprintf("%d too big to have kept", len(done.Skipped)))
	}
	if n, ok := gitChanged(m.root); ok {
		parts = append(parts, fmt.Sprintf("git: %d paths differ from HEAD", n))
	}
	m.notice("%s", strings.Join(parts, " · "))
}

func gitChanged(root string) (int, bool) {
	cmd := exec.Command("git", "status", "--short")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return 0, false
	}
	text := strings.TrimSpace(string(out))
	if text == "" {
		return 0, true
	}
	return len(strings.Split(text, "\n")), true
}
