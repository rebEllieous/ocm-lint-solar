package ruleset

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/rebEllieous/ocm-lint-solar/rules"
)

const (
	urlTimeout  = 30 * time.Second
	maxRuleFile = 1 << 20
)

// Aliases lists the embedded rule files as they are passed to --rule, such as "@solar".
func Aliases() []string {
	entries, err := rules.Files.ReadDir(".")
	if err != nil {
		panic(err) // the embedded root always exists
	}
	var out []string
	for _, e := range entries {
		out = append(out, "@"+strings.TrimSuffix(e.Name(), ".yaml"))
	}
	slices.Sort(out)
	return out
}

// read returns a rule source's content. notice is for the caller to pass on, when
// the source is worth naming in a log. An alias never touches the network.
func read(ctx context.Context, source string) (b []byte, notice string, err error) {
	if name, ok := strings.CutPrefix(source, "@"); ok {
		// Matching against the list means a name like "../core" is simply unknown.
		if !slices.Contains(Aliases(), source) {
			return nil, "", fmt.Errorf("unknown rule alias; available: %s", strings.Join(Aliases(), ", "))
		}
		b, err = rules.Files.ReadFile(name + ".yaml")
		return b, "", err
	}
	if isURL(source) {
		b, err = fetch(ctx, source)
		return b, "loaded rules from " + display(source), err
	}
	if strings.Contains(source, "://") {
		return nil, "", errors.New("unsupported URL scheme; use http:// or https://")
	}
	b, err = os.ReadFile(source)
	return b, "", err
}

func isURL(source string) bool {
	return strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://")
}

// fetch downloads a rule file. Its errors leave out the URL, which can hold a token;
// the caller adds display(source).
func fetch(ctx context.Context, source string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, urlTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, source, nil)
	if err != nil {
		return nil, errors.New("invalid URL")
	}
	client := http.Client{CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return errors.New("stopped after 10 redirects")
		}
		req.Header.Del("Referer") // it holds the URL, which can hold a token
		return nil
	}}
	resp, err := client.Do(req)
	if err != nil {
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err
		}
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %s", resp.Status)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxRuleFile+1))
	if err != nil {
		return nil, err
	}
	if len(b) > maxRuleFile {
		return nil, fmt.Errorf("larger than %d bytes", maxRuleFile)
	}
	return b, nil
}

// display is how a source appears in errors and warnings. URLs lose their user info,
// query and fragment, where tokens go.
func display(source string) string {
	if !strings.Contains(source, "://") {
		return source
	}
	u, err := url.Parse(source)
	if err != nil {
		return "invalid URL"
	}
	u.User, u.RawQuery, u.Fragment = nil, "", ""
	return u.String()
}
