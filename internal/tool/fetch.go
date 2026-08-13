package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/zenodea/zaino/internal/llm"
	"github.com/zenodea/zaino/internal/permission"
)

type Fetch struct {
	client *http.Client
}

type fetchArgs struct {
	URL string `json:"url"`
	Raw bool   `json:"raw,omitempty"`
}

const (
	fetchTimeout  = 30 * time.Second
	maxFetchBytes = 2 << 20
	maxRedirects  = 5
)

func (f *Fetch) Definition() llm.Tool {
	return llm.Tool{
		Name: "fetch",
		Description: "Fetch a URL over HTTP and return it as text. HTML comes back with the " +
			"markup stripped; set raw to keep the response as sent. Use this for documentation, " +
			"API references and anything else whose current wording matters.",
		InputSchema: object(map[string]any{
			"url": field("string", "Absolute http or https URL."),
			"raw": field("boolean", "Return the body as sent instead of stripping HTML."),
		}, "url"),
	}
}

func (f *Fetch) Prepare(input json.RawMessage) (Call, error) {
	args, err := parse[fetchArgs](input)
	if err != nil {
		return nil, err
	}

	parsed, err := url.Parse(strings.TrimSpace(args.URL))
	if err != nil {
		return nil, fmt.Errorf("bad url: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("only http and https are fetched, not %q", parsed.Scheme)
	}
	if parsed.Host == "" {
		return nil, fmt.Errorf("no host in %q", args.URL)
	}

	client := f.client
	if client == nil {
		client = &http.Client{
			Timeout: fetchTimeout,
			CheckRedirect: func(_ *http.Request, via []*http.Request) error {
				if len(via) >= maxRedirects {
					return fmt.Errorf("stopped after %d redirects", maxRedirects)
				}
				return nil
			},
		}
	}
	return &fetchCall{client: client, url: parsed, raw: args.Raw}, nil
}

type fetchCall struct {
	client *http.Client
	url    *url.URL
	raw    bool
}

func (c *fetchCall) Request() permission.Request {
	return permission.Request{
		Tool:    "fetch",
		Action:  permission.Network,
		Target:  c.url.Host,
		Preview: c.url.String(),
	}
}

func (c *fetchCall) Run(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url.String(), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("accept", "text/html,text/plain,application/json;q=0.9,*/*;q=0.8")

	resp, err := c.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxFetchBytes))
	if err != nil {
		return "", err
	}
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("%s said %s\n%s", c.url.Host, resp.Status,
			clipOutput(strings.TrimSpace(string(body))))
	}
	if isBinary(body) {
		return "", fmt.Errorf("%s returned %s, which is not text", c.url.Host, resp.Header.Get("content-type"))
	}

	text := string(body)
	if !c.raw && strings.Contains(resp.Header.Get("content-type"), "html") {
		text = htmlToText(text)
	}

	head := fmt.Sprintf("%s · %s · %s\n\n", c.url.String(), resp.Status, fmt.Sprintf("%d bytes", len(body)))
	return head + clipOutput(strings.TrimSpace(text)), nil
}

// RE2 has no backreferences, so each container is its own pattern.
var dropped = []*regexp.Regexp{
	regexp.MustCompile(`(?is)<script\b[^>]*>.*?</script\s*>`),
	regexp.MustCompile(`(?is)<style\b[^>]*>.*?</style\s*>`),
	regexp.MustCompile(`(?is)<noscript\b[^>]*>.*?</noscript\s*>`),
	regexp.MustCompile(`(?is)<svg\b[^>]*>.*?</svg\s*>`),
	regexp.MustCompile(`(?is)<head\b[^>]*>.*?</head\s*>`),
	regexp.MustCompile(`(?s)<!--.*?-->`),
}

var (
	title     = regexp.MustCompile(`(?is)<title\b[^>]*>(.*?)</title\s*>`)
	breaking  = regexp.MustCompile(`(?i)</?(p|div|br|li|tr|h[1-6]|section|article|header|footer|pre)\b[^>]*>`)
	anyTag    = regexp.MustCompile(`(?s)<[^>]*>`)
	manyLines = regexp.MustCompile(`\n{3,}`)
	manySpace = regexp.MustCompile(`[ \t]{2,}`)
)

var entities = strings.NewReplacer(
	"&nbsp;", " ", "&amp;", "&", "&lt;", "<", "&gt;", ">",
	"&quot;", `"`, "&#39;", "'", "&apos;", "'", "&mdash;", "—", "&ndash;", "–",
)

// Enough to read prose with. A real parser would be better, and would be a
// dependency; what the model needs from a docs page is the words.
func htmlToText(html string) string {
	heading := ""
	if m := title.FindStringSubmatch(html); len(m) == 2 {
		heading = strings.TrimSpace(entities.Replace(anyTag.ReplaceAllString(m[1], ""))) + "\n\n"
	}
	for _, re := range dropped {
		html = re.ReplaceAllString(html, " ")
	}
	html = breaking.ReplaceAllString(html, "\n")
	html = anyTag.ReplaceAllString(html, "")
	html = entities.Replace(html)

	lines := strings.Split(html, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimSpace(manySpace.ReplaceAllString(line, " "))
	}
	return heading + manyLines.ReplaceAllString(strings.Join(lines, "\n"), "\n\n")
}
