package antcli

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
)

func stub(out string, err error) (*CLI, *[][]string) {
	var calls [][]string
	c := &CLI{Path: "/fake/ant"}
	c.run = func(_ context.Context, name string, args ...string) ([]byte, error) {
		calls = append(calls, append([]string{name}, args...))
		return []byte(out), err
	}
	return c, &calls
}

func TestAccessTokenAsksForTheBareToken(t *testing.T) {
	c, calls := stub("sk-ant-oat-abc123\n", nil)

	got, err := c.AccessToken(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got != "sk-ant-oat-abc123" {
		t.Errorf("token = %q", got)
	}

	if len(*calls) != 1 {
		t.Fatalf("made %d calls", len(*calls))
	}
	// Without the flag the CLI prints the whole credentials JSON.
	if !slices.Contains((*calls)[0], "--access-token") {
		t.Errorf("call = %v, want --access-token", (*calls)[0])
	}
}

func TestAccessTokenRefusesTheCredentialsJSON(t *testing.T) {
	c, _ := stub(`{"access_token":"abc","refresh_token":"def"}`, nil)

	_, err := c.AccessToken(context.Background())
	if err == nil {
		t.Fatal("got nil, want an error")
	}
	if !strings.Contains(err.Error(), "bare token") {
		t.Errorf("got %v", err)
	}
}

func TestAccessTokenWithNoProfile(t *testing.T) {
	c, _ := stub("  \n", nil)

	_, err := c.AccessToken(context.Background())
	if !errors.Is(err, ErrNoProfile) {
		t.Fatalf("got %v, want ErrNoProfile", err)
	}
}

func TestAccessTokenSurfacesTheCLIError(t *testing.T) {
	c, _ := stub("", errors.New("not logged in"))

	if _, err := c.AccessToken(context.Background()); err == nil {
		t.Fatal("got nil, want an error")
	}
}

func TestAProfileIsPassedThrough(t *testing.T) {
	c, calls := stub("token\n", nil)
	c.Profile = "work"

	if _, err := c.AccessToken(context.Background()); err != nil {
		t.Fatal(err)
	}
	call := (*calls)[0]
	if !slices.Contains(call, "--profile") || !slices.Contains(call, "work") {
		t.Errorf("call = %v, want the profile", call)
	}
	if call[1] != "--profile" {
		t.Errorf("the profile flag must precede the subcommand: %v", call)
	}
}

func TestStatus(t *testing.T) {
	c, calls := stub("active profile: default\n", nil)

	got, err := c.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got != "active profile: default" {
		t.Errorf("status = %q", got)
	}
	if !slices.Contains((*calls)[0], "status") {
		t.Errorf("call = %v", (*calls)[0])
	}
}

func TestLoggedIn(t *testing.T) {
	c, _ := stub("token\n", nil)
	if !c.LoggedIn(context.Background()) {
		t.Error("LoggedIn = false with a token")
	}

	c, _ = stub("", errors.New("nope"))
	if c.LoggedIn(context.Background()) {
		t.Error("LoggedIn = true without a token")
	}
}

func TestAMissingBinaryIsItsOwnError(t *testing.T) {
	c := &CLI{}
	c.run = func(context.Context, string, ...string) ([]byte, error) {
		t.Fatal("the CLI was run despite being absent")
		return nil, nil
	}
	if c.Installed() {
		t.Skip("ant is installed on this machine")
	}

	if _, err := c.AccessToken(context.Background()); !errors.Is(err, ErrNotInstalled) {
		t.Errorf("AccessToken: got %v, want ErrNotInstalled", err)
	}
	if _, err := c.Status(context.Background()); !errors.Is(err, ErrNotInstalled) {
		t.Errorf("Status: got %v, want ErrNotInstalled", err)
	}
}

func TestInstalledFollowsAnExplicitPath(t *testing.T) {
	if !(&CLI{Path: "/anywhere/ant"}).Installed() {
		t.Error("an explicit path should count as installed")
	}
}

func TestLoginArgs(t *testing.T) {
	c := &CLI{Path: "/fake/ant"}

	got := c.LoginArgs(true)
	want := []string{"/fake/ant", "auth", "login"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("got %v, want %v", got, want)
	}

	// A machine with no browser gets the code back in the terminal.
	if got := c.LoginArgs(false); !slices.Contains(got, "--no-browser") {
		t.Errorf("got %v, want --no-browser", got)
	}

	c.Profile = "work"
	if got := c.LoginArgs(true); !slices.Contains(got, "work") {
		t.Errorf("got %v, want the profile", got)
	}
}

func TestInstallHintNamesAWayToGetIt(t *testing.T) {
	if !strings.Contains(InstallHint(), "brew") {
		t.Errorf("got %q", InstallHint())
	}
}
