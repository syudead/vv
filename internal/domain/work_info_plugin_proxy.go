package domain

import (
	"errors"
	"net/url"
	"strconv"
	"strings"
)

const WorkInfoPluginProxyURLMaxLen = 2048

var ErrWorkInfoPluginProxyURLInvalid = errors.New("work-info plugin proxy URL is invalid")

var proxySchemes = map[string]bool{"http": true, "https": true, "socks5": true}

// WorkInfoRunNetwork's zero value uses no proxy.
type WorkInfoRunNetwork struct {
	Proxy *url.URL
}

// ParseWorkInfoPluginProxyURL accepts bounded HTTP(S)/SOCKS5 URLs with host and optional port/credentials but no path, query, or fragment; empty means no proxy.
func ParseWorkInfoPluginProxyURL(raw string) (*url.URL, error) {
	if raw == "" {
		return nil, nil
	}
	if !runeCountBetween(raw, 1, WorkInfoPluginProxyURLMaxLen) {
		return nil, ErrWorkInfoPluginProxyURLInvalid
	}
	// Reject bare fragment/port delimiters in raw input because url.Parse treats them like absence.
	if strings.Contains(raw, "#") {
		return nil, ErrWorkInfoPluginProxyURLInvalid
	}
	u, err := url.Parse(raw)
	if err != nil || strings.HasSuffix(u.Host, ":") || !proxySchemes[u.Scheme] || u.Opaque != "" || u.Hostname() == "" ||
		u.Path != "" || u.RawPath != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.RawFragment != "" ||
		!validProxyPort(u.Port()) {
		return nil, ErrWorkInfoPluginProxyURLInvalid
	}
	return u, nil
}

func validProxyPort(port string) bool {
	if port == "" {
		return true
	}
	n, err := strconv.Atoi(port)
	return err == nil && n >= 1 && n <= 65535
}
