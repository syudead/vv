package domain

import (
	"errors"
	"net/url"
	"strings"
)

// A person plugin that declares domains fetches a person by one of the person's links under them
// (docs/design-docs/person-plugins.md).

var (
	// ErrPersonLinkUnsupported reports a link whose host matches none of the person plugin's domains.
	ErrPersonLinkUnsupported = errors.New("no domain of the person plugin matches the link")
	// ErrPersonLinkAmbiguous reports a person with several folded links the person plugin handles; see
	// PersonLinkAmbiguousError.
	ErrPersonLinkAmbiguous = errors.New("several of the person's links match the person plugin's domains")
)

// PersonLinkAmbiguousError carries the folded matching links in stored order and matches ErrPersonLinkAmbiguous.
type PersonLinkAmbiguousError struct{ Links []string }

func (e *PersonLinkAmbiguousError) Error() string { return ErrPersonLinkAmbiguous.Error() }

func (e *PersonLinkAmbiguousError) Unwrap() error { return ErrPersonLinkAmbiguous }

// LinkMatchesDomains reports whether link is an http or https URL whose lowercased host is one of domains or ends
// with "." and one of them.
func LinkMatchesDomains(link string, domains []string) bool {
	parsed, err := url.Parse(link)
	if err != nil || !isWorkURL(link) {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	for _, domain := range domains {
		if host == domain || strings.HasSuffix(host, "."+domain) {
			return true
		}
	}
	return false
}

// PersonFetchLinks returns the links that match domains, in stored order, keeping only the first of links that
// differ only in scheme, host case or a trailing slash; query strings and fragments tell links apart.
func PersonFetchLinks(links, domains []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, link := range links {
		if !LinkMatchesDomains(link, domains) {
			continue
		}
		key := foldedPersonLink(link)
		if !seen[key] {
			seen[key] = true
			out = append(out, link)
		}
	}
	return out
}

// foldedPersonLink expects a link LinkMatchesDomains accepted.
func foldedPersonLink(link string) string {
	parsed, _ := url.Parse(link)
	var b strings.Builder
	if parsed.User != nil {
		b.WriteString(parsed.User.String() + "@")
	}
	b.WriteString(strings.ToLower(parsed.Host))
	b.WriteString(strings.TrimSuffix(parsed.EscapedPath(), "/"))
	if parsed.ForceQuery || parsed.RawQuery != "" {
		b.WriteString("?" + parsed.RawQuery)
	}
	if parsed.Fragment != "" {
		b.WriteString("#" + parsed.EscapedFragment())
	}
	return b.String()
}

// NormalizePersonFetchLink trims a link sent for a fetch and checks it as a stored person link is checked; a value
// that is not an http or https URL returns ErrWorkInfoRunInvalid.
func NormalizePersonFetchLink(raw string) (string, error) {
	link, err := normalizeWorkInfoText("link", raw, WorkInfoLimits.URLMaxLength, false)
	if err != nil || !isWorkURL(link) {
		return "", ErrWorkInfoRunInvalid
	}
	return link, nil
}
