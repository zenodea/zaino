package tui

import (
	"context"
	"os/exec"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/zenodea/zaino/internal/provider"
	"github.com/zenodea/zaino/internal/store/credentials"
	"github.com/zenodea/zaino/internal/x/antcli"
)

// apiKeySource says where a provider's key comes from, so the entry screen is not
// a blank box.
var apiKeySource = map[string]string{
	"anthropic":  "console.anthropic.com · starts with sk-ant-",
	"gemini":     "aistudio.google.com/apikey",
	"openai":     "platform.openai.com/api-keys · starts with sk-",
	"grok":       "console.x.ai · starts with xai-",
	"openrouter": "openrouter.ai/keys · starts with sk-or-",
}

// oauthProviders are the ones with a login flow. OpenAI and xAI offer no
// OAuth for API access, so a key is the only way in.
var oauthProviders = map[string]bool{"anthropic": true}

type loginDoneMsg struct {
	provider string
	err      error
}

// setUpProvider offers the ways of authenticating that this provider has.
func (m *Model) setUpProvider(name string) tea.Cmd {
	var options []choice

	if oauthProviders[name] {
		cli := antcli.New()
		detail, body := "opens a browser, no key to store",
			[]string{
				hintStyle.Render("Hands the terminal to `ant auth login`, which does the"),
				hintStyle.Render("OAuth flow and holds the tokens. zaino stores no secret."),
			}
		if !cli.Installed() {
			detail = "needs the ant CLI"
			body = []string{
				hintStyle.Render("The ant CLI is not installed."),
				hintStyle.Render(antcli.InstallHint()),
			}
		}
		options = append(options, choice{
			label: "log in", detail: detail, value: "oauth", body: body,
		})
	}

	options = append(options, choice{
		label:  "paste an API key",
		detail: apiKeySource[name],
		value:  "key",
		body: []string{
			hintStyle.Render("Typed into a masked field, never echoed and never"),
			hintStyle.Render("written to the transcript or the session file."),
			"",
			hintStyle.Render("Stored at " + m.credentialsPath() + " (owner only)."),
		},
	})

	options = append(options, choice{
		label: "cancel", detail: "leave " + name + " alone", value: "cancel",
	})

	return m.ask(chooser{
		title:   name + " · not set up",
		options: options,
		apply: func(m *Model, picked choice) {
			switch picked.value {
			case "oauth":
				m.startLogin(name)
			case "key":
				m.runCmd(m.enterKey(name))
			}
		},
	})
}

func (m *Model) enterKey(name string) tea.Cmd {
	return m.askSecret(name+" API key", apiKeySource[name],
		func(m *Model, value string) tea.Cmd {
			store := m.credentials()
			if store == nil {
				m.push(entry{kind: entryError, text: "credentials cannot be stored here"})
				return nil
			}
			if err := store.SetKey(name, value); err != nil {
				m.push(entry{kind: entryError, text: err.Error()})
				return nil
			}
			m.notice("%s key saved · %s", name, store.Path())
			return cmdProvider(m, name)
		})
}

// startLogin hands the terminal to the ant CLI: the flow is interactive and
// opens a browser, so the UI has to step aside for it.
func (m *Model) startLogin(name string) {
	cli := antcli.New()
	if !cli.Installed() {
		m.push(entry{kind: entryError, text: "the ant CLI is not installed\n" + antcli.InstallHint()})
		return
	}

	args := cli.LoginArgs(true)
	cmd := exec.Command(args[0], args[1:]...)
	m.runCmd(tea.ExecProcess(cmd, func(err error) tea.Msg {
		return loginDoneMsg{provider: name, err: err}
	}))
}

func (m *Model) finishLogin(msg loginDoneMsg) tea.Cmd {
	if msg.err != nil {
		m.push(entry{kind: entryError, text: "login: " + msg.err.Error()})
		return nil
	}

	cli := antcli.New()
	if !cli.LoggedIn(context.Background()) {
		m.push(entry{kind: entryError, text: "login did not leave a usable profile"})
		return nil
	}
	if store := m.credentials(); store != nil {
		if err := store.SetOAuth(msg.provider, ""); err != nil {
			m.push(entry{kind: entryError, text: err.Error()})
			return nil
		}
	}
	m.notice("%s · logged in", msg.provider)
	return cmdProvider(m, msg.provider)
}

func (m *Model) credentials() *credentials.Store {
	if m.creds == nil {
		store, err := credentials.Open()
		if err != nil {
			return nil
		}
		m.creds = store
		provider.SetStore(store)
	}
	return m.creds
}

func (m *Model) credentialsPath() string {
	if store := m.credentials(); store != nil {
		return store.Path()
	}
	return "the credentials file"
}

// credentialState is the one-line summary the provider picker shows.
func credentialState(name string) string {
	if provider.HasCredentials(name) {
		return "ready"
	}
	if oauthProviders[name] {
		return "not set up · log in or paste a key"
	}
	return "not set up · paste a key"
}
