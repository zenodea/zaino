package tui

import (
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/zenodea/zaino/internal/provider"
	"github.com/zenodea/zaino/internal/store/credentials"
)

func typeInto(m *Model, s string) {
	for _, r := range s {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
}

func isolate(t *testing.T) *Model {
	t.Helper()
	root := t.TempDir()
	t.Setenv("XDG_DATA_HOME", root)
	for _, key := range []string{
		"ANTHROPIC_API_KEY", "ANTHROPIC_AUTH_TOKEN",
		"GEMINI_API_KEY", "GOOGLE_API_KEY",
		"OPENAI_API_KEY", "XAI_API_KEY", "GROK_API_KEY",
	} {
		t.Setenv(key, "")
	}
	provider.SetStore(nil)
	t.Cleanup(func() { provider.SetStore(nil) })

	m := newTestModel(t, 80, 24)
	m.creds = credentials.At(filepath.Join(root, credentials.FileName))
	provider.SetStore(m.creds)
	return m
}

func TestASecretIsNotEchoed(t *testing.T) {
	m := isolate(t)

	m.Update(m.askSecret("openai API key", "hint", nil))
	m.askSecret("openai API key", "hint", nil)
	typeInto(m, "sk-supersecret")

	view := m.View()
	if strings.Contains(view, "sk-supersecret") {
		t.Errorf("the key is on screen:\n%s", view)
	}
	if !strings.Contains(view, "•") {
		t.Errorf("the field is not masked:\n%s", view)
	}
	if got := m.secret.input.Value(); got != "sk-supersecret" {
		t.Errorf("the field did not receive the key: %q", got)
	}
}

func TestASecretNeverBecomesAMessage(t *testing.T) {
	m := isolate(t)
	m.askSecret("openai API key", "", nil)

	typeInto(m, "sk-supersecret")
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	for _, msg := range m.messages {
		if strings.Contains(msg.Text(), "sk-supersecret") {
			t.Fatal("the key reached the conversation")
		}
	}
	for _, e := range m.entries {
		if strings.Contains(e.text, "sk-supersecret") {
			t.Fatalf("the key reached the transcript: %+v", e)
		}
	}
}

func TestEnteringAKeyStoresItAndSwitches(t *testing.T) {
	m := isolate(t)

	m.enterKey("openai")
	typeInto(m, "sk-entered")
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	entry, ok, err := m.creds.Lookup("openai")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("nothing was stored")
	}
	if entry.Key != "sk-entered" || entry.Method != credentials.APIKey {
		t.Errorf("stored %+v", entry)
	}
	if m.provider != "openai" {
		t.Errorf("provider = %q, want openai after setting it up", m.provider)
	}
}

func TestEscapeAbandonsTheKey(t *testing.T) {
	m := isolate(t)

	m.enterKey("openai")
	typeInto(m, "sk-entered")
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})

	if m.secret.open {
		t.Error("the field is still open")
	}
	if _, ok, _ := m.creds.Lookup("openai"); ok {
		t.Error("a cancelled key was stored anyway")
	}
}

func TestAnEmptyKeyIsNotStored(t *testing.T) {
	m := isolate(t)

	m.enterKey("openai")
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if _, ok, _ := m.creds.Lookup("openai"); ok {
		t.Error("an empty key was stored")
	}
}

// While a key is being typed, nothing else may act on the keystrokes.
func TestTheSecretFieldSwallowsOtherBindings(t *testing.T) {
	m := isolate(t)
	before := len(m.entries)

	m.enterKey("openai")
	for _, key := range []string{"j", "k", "q", "g"} {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
	}

	if !m.secret.open {
		t.Fatal("the field closed")
	}
	if got := m.secret.input.Value(); got != "jkqg" {
		t.Errorf("the field received %q", got)
	}
	if len(m.entries) != before {
		t.Error("a keystroke reached the transcript")
	}
}

func TestSettingUpOffersLoginOnlyWhereItExists(t *testing.T) {
	m := isolate(t)

	m.setUpProvider("anthropic")
	if !hasOption(m, "log in") {
		t.Errorf("anthropic has oauth but no login option: %v", optionValues(m))
	}
	m.closeChooser()

	m.setUpProvider("openai")
	if hasOption(m, "log in") {
		t.Errorf("openai has no oauth for api access: %v", optionValues(m))
	}
	if !hasOption(m, "paste an API key") {
		t.Errorf("no way to set openai up: %v", optionValues(m))
	}
}

func TestEveryProviderCanBeSetUp(t *testing.T) {
	m := isolate(t)

	for _, name := range provider.Available() {
		m.setUpProvider(name)
		if !hasOption(m, "paste an API key") {
			t.Errorf("%s offers no way in: %v", name, optionValues(m))
		}
		m.closeChooser()
	}
}

// Picking an unset-up provider should open the setup screen, not just report
// that there is no key.
func TestChoosingAnUnconfiguredProviderOffersSetup(t *testing.T) {
	m := isolate(t)

	cmdProvider(m, "grok")
	if !m.chooser.open {
		t.Fatal("no setup screen opened")
	}
	if !strings.Contains(m.chooser.title, "grok") {
		t.Errorf("title = %q", m.chooser.title)
	}
}

func TestAnUnknownProviderIsStillAnError(t *testing.T) {
	m := isolate(t)
	before := len(m.entries)

	cmdProvider(m, "llama")
	if m.chooser.open {
		t.Error("an unknown provider opened a setup screen")
	}
	if len(m.entries) == before {
		t.Error("nothing was reported")
	}
}

func TestCredentialState(t *testing.T) {
	m := isolate(t)
	_ = m

	if got := credentialState("openai"); !strings.Contains(got, "not set up") {
		t.Errorf("got %q", got)
	}

	t.Setenv("OPENAI_API_KEY", "sk-test")
	if got := credentialState("openai"); got != "ready" {
		t.Errorf("got %q, want ready", got)
	}
}

func TestAStoredKeyMakesTheProviderReady(t *testing.T) {
	m := isolate(t)
	if err := m.creds.SetKey("grok", "xai-stored"); err != nil {
		t.Fatal(err)
	}

	if got := credentialState("grok"); got != "ready" {
		t.Errorf("got %q, want ready — the store was not consulted", got)
	}
}

func hasOption(m *Model, label string) bool {
	for _, o := range m.chooser.options {
		if o.label == label {
			return true
		}
	}
	return false
}

func optionValues(m *Model) []string {
	out := make([]string, len(m.chooser.options))
	for i, o := range m.chooser.options {
		out[i] = o.label
	}
	return out
}

func TestEveryProviderHasAKeySource(t *testing.T) {
	for _, name := range provider.Available() {
		if apiKeySource[name] == "" {
			t.Errorf("%s offers no hint about where its key comes from", name)
		}
	}
}
