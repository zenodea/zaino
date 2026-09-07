package tool

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

type job struct {
	id      string
	command string
	cmd     *exec.Cmd
	logPath string
	started time.Time
	done    chan struct{}
	err     error
}

type JobInfo struct {
	ID      string
	Command string
	PID     int
	Started time.Time
	Running bool
	Status  string
}

type jobTable struct {
	mu    sync.Mutex
	next  int
	byID  map[string]*job
	order []*job
}

var jobs = &jobTable{byID: map[string]*job{}}

func (t *jobTable) start(dir, command string) (*job, error) {
	log, err := os.CreateTemp("", "zaino-job-*.log")
	if err != nil {
		return nil, err
	}
	cmd := exec.Command("sh", "-c", command)
	cmd.Dir = dir
	cmd.Stdout, cmd.Stderr = log, log
	setpgid(cmd)
	if err := cmd.Start(); err != nil {
		log.Close()
		os.Remove(log.Name())
		return nil, err
	}

	t.mu.Lock()
	t.next++
	j := &job{
		id: fmt.Sprintf("j%d", t.next), command: command, cmd: cmd,
		logPath: log.Name(), started: time.Now(), done: make(chan struct{}),
	}
	t.byID[j.id] = j
	t.order = append(t.order, j)
	t.mu.Unlock()

	go func() {
		j.err = cmd.Wait()
		log.Close()
		close(j.done)
	}()
	return j, nil
}

func (t *jobTable) get(id string) (*job, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	j, ok := t.byID[strings.TrimSpace(id)]
	return j, ok
}

func (t *jobTable) all() []*job {
	t.mu.Lock()
	defer t.mu.Unlock()
	return append([]*job(nil), t.order...)
}

func (j *job) running() bool {
	select {
	case <-j.done:
		return false
	default:
		return true
	}
}

func (j *job) status() string {
	if j.running() {
		return fmt.Sprintf("running (pid %d)", j.cmd.Process.Pid)
	}
	if j.err == nil {
		return "exited 0"
	}
	return j.err.Error()
}

func (j *job) output(lines int) (string, error) {
	data, err := os.ReadFile(j.logPath)
	if err != nil {
		return "", err
	}
	text := strings.TrimRight(string(data), "\n")
	if lines > 0 {
		all := strings.Split(text, "\n")
		if len(all) > lines {
			text = strings.Join(all[len(all)-lines:], "\n")
		}
	}
	return clipTail(text, maxOutputRune), nil
}

func (j *job) kill() error {
	if !j.running() {
		return nil
	}
	if err := terminate(j.cmd, false); err != nil {
		return err
	}
	select {
	case <-j.done:
		return nil
	case <-time.After(2 * time.Second):
	}
	if err := terminate(j.cmd, true); err != nil {
		return err
	}
	select {
	case <-j.done:
	case <-time.After(time.Second):
	}
	return nil
}

func (j *job) info() JobInfo {
	return JobInfo{
		ID: j.id, Command: j.command, PID: j.cmd.Process.Pid, Started: j.started,
		Running: j.running(), Status: j.status(),
	}
}

func Jobs() []JobInfo {
	all := jobs.all()
	out := make([]JobInfo, len(all))
	for i, j := range all {
		out[i] = j.info()
	}
	return out
}

func StopJobs() {
	for _, j := range jobs.all() {
		j.kill()
		os.Remove(j.logPath)
	}
}

func clipTail(s string, limit int) string {
	runes := []rune(s)
	if len(runes) <= limit {
		return s
	}
	return fmt.Sprintf("… earlier output cut, showing the last %d characters\n", limit) + string(runes[len(runes)-limit:])
}
