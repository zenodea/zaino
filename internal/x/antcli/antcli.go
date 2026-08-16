// Package antcli borrows credentials from the official Anthropic CLI.
//
// Logging in is an OAuth flow with a browser redirect, a token exchange and a
// refresh cycle. `ant` already implements all of it and stores the profile
// under ~/.config/anthropic, so zaino asks it for a token rather than running
// a second, competing OAuth client.
package antcli

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

const Binary = "ant"

var ErrNotInstalled = errors.New("antcli: the ant CLI is not installed")

var ErrNoProfile = errors.New("antcli: no profile is logged in")

type CLI struct {
	// Path to the binary. Empty means look "ant" up on PATH.
	Path string

	// Profile picks a named profile; empty uses the active one.
	Profile string

	// run executes the CLI. Tests replace it.
	run func(ctx context.Context, name string, args ...string) ([]byte, error)
}

func New() *CLI { return &CLI{} }

func (c *CLI) binary() string {
	if c.Path != "" {
		return c.Path
	}
	return Binary
}

func (c *CLI) exec(ctx context.Context, args ...string) ([]byte, error) {
	if c.Profile != "" {
		args = append([]string{"--profile", c.Profile}, args...)
	}
	if c.run != nil {
		return c.run(ctx, c.binary(), args...)
	}
	out, err := exec.CommandContext(ctx, c.binary(), args...).Output()
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) && len(exit.Stderr) > 0 {
			return nil, fmt.Errorf("antcli: %s: %s", strings.Join(args, " "),
				strings.TrimSpace(string(exit.Stderr)))
		}
		return nil, fmt.Errorf("antcli: %s: %w", strings.Join(args, " "), err)
	}
	return out, nil
}

// Installed reports whether the CLI can be found at all.
func (c *CLI) Installed() bool {
	if c.Path != "" {
		return true
	}
	_, err := exec.LookPath(Binary)
	return err == nil
}

// AccessToken returns a short-lived bearer token, refreshing it if the stored
// one has expired. The bare `print-credentials` prints the whole credentials
// JSON, so the flag is not optional.
func (c *CLI) AccessToken(ctx context.Context) (string, error) {
	if !c.Installed() {
		return "", ErrNotInstalled
	}
	out, err := c.exec(ctx, "auth", "print-credentials", "--access-token")
	if err != nil {
		return "", err
	}
	token := strings.TrimSpace(string(out))
	if token == "" {
		return "", ErrNoProfile
	}
	// Guard against the no-flag form, whose JSON would go out as a bearer
	// token and come back as an unreadable protocol error.
	if strings.HasPrefix(token, "{") {
		return "", errors.New("antcli: expected a bare token, got the credentials json")
	}
	return token, nil
}

// Status is what `ant auth status` prints: which credential source and profile
// won. It reports status only, so its exit code is not a health check.
func (c *CLI) Status(ctx context.Context) (string, error) {
	if !c.Installed() {
		return "", ErrNotInstalled
	}
	out, err := c.exec(ctx, "auth", "status")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// LoggedIn reports whether a token can be had right now.
func (c *CLI) LoggedIn(ctx context.Context) bool {
	_, err := c.AccessToken(ctx)
	return err == nil
}

// LoginArgs is the command a frontend should run to log in. It is not run
// here: it takes over the terminal and opens a browser.
func (c *CLI) LoginArgs(browser bool) []string {
	args := []string{c.binary(), "auth", "login"}
	if c.Profile != "" {
		args = append(args, "--profile", c.Profile)
	}
	if !browser {
		args = append(args, "--no-browser")
	}
	return args
}

func InstallHint() string {
	return "install it with: brew install anthropics/tap/ant\n" +
		"  or see github.com/anthropics/anthropic-cli/releases"
}
