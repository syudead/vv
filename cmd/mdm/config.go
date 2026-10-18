package main

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"unicode"
)

// Configuration comes only from environment variables; see README.md for defaults.
const (
	envAddr     = "MDM_ADDR"
	envDataDir  = "MDM_DATA_DIR"
	envLogLevel = "MDM_LOG_LEVEL"
	// Unset trusts loopback and private proxies; "none" disables forwarded headers.
	envTrustedProxies = "MDM_TRUSTED_PROXIES"
	// Empty uses the bundled workflow model; overrides allow compatible variants.
	envComfyUIDiffusionModel = "MDM_COMFYUI_DIFFUSION_MODEL"
)

const (
	defaultAddr     = ":8080"
	defaultDataDir  = "/data"
	defaultLogLevel = "info"
)

// Trust LAN and Docker proxies by default. LAN peers can spoof forwarded addresses, while internet
// requests use the proxy-supplied address.
var defaultTrustedProxies = []netip.Prefix{
	netip.MustParsePrefix("127.0.0.0/8"),
	netip.MustParsePrefix("10.0.0.0/8"),
	netip.MustParsePrefix("172.16.0.0/12"),
	netip.MustParsePrefix("192.168.0.0/16"),
	netip.MustParsePrefix("::1/128"),
	netip.MustParsePrefix("fc00::/7"),
}

const noTrustedProxies = "none"

const dataDirPerm os.FileMode = 0o755

// Keep thumbnails under MDM_DATA_DIR rather than exposing another setting.
const thumbnailsDirName = "thumbnails"

// Config is immutable after startup; the database path is fixed at DataDir/mdm.db.
type Config struct {
	Addr     string
	DataDir  string
	LogLevel string
	// Empty disables forwarded headers; unset uses defaultTrustedProxies.
	TrustedProxies []netip.Prefix
	// Empty leaves the workflow model unchanged.
	ComfyUIDiffusionModel string
}

var logLevels = map[string]slog.Level{
	"debug": slog.LevelDebug,
	"info":  slog.LevelInfo,
	"warn":  slog.LevelWarn,
	"error": slog.LevelError,
}

// LoadConfig validates environment values; Verify checks filesystem prerequisites. Collect all
// errors so one correction cycle can fix them together.
func LoadConfig(getenv func(string) string) (Config, error) {
	cfg := Config{
		Addr:     valueOr(getenv(envAddr), defaultAddr),
		DataDir:  valueOr(getenv(envDataDir), defaultDataDir),
		LogLevel: valueOr(getenv(envLogLevel), defaultLogLevel),

		ComfyUIDiffusionModel: strings.TrimSpace(getenv(envComfyUIDiffusionModel)),
	}

	var problems []error

	if _, _, err := net.SplitHostPort(cfg.Addr); err != nil {
		problems = append(problems, fmt.Errorf(
			"%s=%q is not a valid listen address (for example :8080 or 127.0.0.1:8080): %w",
			envAddr, cfg.Addr, err))
	}

	if !filepath.IsAbs(cfg.DataDir) {
		problems = append(problems, fmt.Errorf(
			"%s=%q is not an absolute path", envDataDir, cfg.DataDir))
	}

	if _, ok := logLevels[cfg.LogLevel]; !ok {
		problems = append(problems, fmt.Errorf(
			"%s=%q is not a known value (use debug, info, warn or error)",
			envLogLevel, cfg.LogLevel))
	}

	proxies, proxyProblems := parseTrustedProxies(getenv(envTrustedProxies))
	cfg.TrustedProxies = proxies
	problems = append(problems, proxyProblems...)

	if len(problems) > 0 {
		return cfg, joinProblems("invalid configuration", problems)
	}
	return cfg, nil
}

// Verify checks filesystem prerequisites, creates DataDir if needed, and collects all errors.
func (c Config) Verify() error {
	problems := c.verifyProblems()
	if len(problems) > 0 {
		return joinProblems("configuration check failed", problems)
	}
	return nil
}

// Return individual problems so startup can report them together with external-command failures.
func (c Config) verifyProblems() []error {
	var problems []error

	if err := os.MkdirAll(c.DataDir, dataDirPerm); err != nil {
		problems = append(problems, fmt.Errorf("cannot create %s=%s: %w", envDataDir, c.DataDir, err))
	} else if err := checkReadableDir(c.DataDir); err != nil {
		problems = append(problems, fmt.Errorf("cannot read %s=%s: %w", envDataDir, c.DataDir, err))
	}

	// Check the thumbnail directory at startup so missing images cannot first surface halfway through
	// ingestion.
	if err := os.MkdirAll(c.ThumbnailsDir(), dataDirPerm); err != nil {
		problems = append(problems, fmt.Errorf(
			"cannot create the thumbnail directory %s: %w", c.ThumbnailsDir(), err))
	}

	return problems
}

// LoadConfig guarantees a known level.
func (c Config) Level() slog.Level {
	if level, ok := logLevels[c.LogLevel]; ok {
		return level
	}
	return slog.LevelInfo
}

func (c Config) LogAttrs() []slog.Attr {
	return []slog.Attr{
		slog.String(envAddr, c.Addr),
		slog.String(envDataDir, c.DataDir),
		slog.String(envLogLevel, c.LogLevel),
		slog.String(envTrustedProxies, formatPrefixes(c.TrustedProxies)),
		slog.String(envComfyUIDiffusionModel, c.ComfyUIDiffusionModel),
	}
}

// parseTrustedProxies accepts comma/space-separated CIDRs or addresses, defaults on empty input,
// and disables trust on "none"; it collects all invalid entries.
func parseTrustedProxies(value string) ([]netip.Prefix, []error) {
	switch trimmed := strings.TrimSpace(value); {
	case trimmed == "":
		return slices.Clone(defaultTrustedProxies), nil
	case strings.EqualFold(trimmed, noTrustedProxies):
		return nil, nil
	}
	var (
		prefixes []netip.Prefix
		problems []error
	)
	fields := strings.FieldsFunc(value, func(r rune) bool { return r == ',' || unicode.IsSpace(r) })
	for _, field := range fields {
		prefix, err := parseTrustedProxy(field)
		if err != nil {
			problems = append(problems, fmt.Errorf(
				"%s: %q is not a valid CIDR (for example 127.0.0.1/32, 10.0.0.0/8 or ::1/128; use %s to ignore forwarding headers)",
				envTrustedProxies, field, noTrustedProxies))
			continue
		}
		prefixes = append(prefixes, prefix)
	}
	return prefixes, problems
}

func parseTrustedProxy(field string) (netip.Prefix, error) {
	if prefix, err := netip.ParsePrefix(field); err == nil {
		return normalizePrefix(prefix), nil
	}
	addr, err := netip.ParseAddr(field)
	if err != nil || addr.Zone() != "" {
		return netip.Prefix{}, errors.New("not a CIDR")
	}
	return normalizePrefix(netip.PrefixFrom(addr, addr.BitLen())), nil
}

// Normalize mapped IPv6 prefixes to IPv4 to match normalized client addresses.
func normalizePrefix(prefix netip.Prefix) netip.Prefix {
	addr := prefix.Addr()
	if addr.Is4In6() && prefix.Bits() >= 96 {
		prefix = netip.PrefixFrom(addr.Unmap(), prefix.Bits()-96)
	}
	return prefix.Masked()
}

func formatPrefixes(prefixes []netip.Prefix) string {
	parts := make([]string, 0, len(prefixes))
	for _, prefix := range prefixes {
		parts = append(parts, prefix.String())
	}
	return strings.Join(parts, ",")
}

// Keep artifacts together to simplify backup and deletion.
func (c Config) ThumbnailsDir() string {
	return filepath.Join(c.DataDir, thumbnailsDirName)
}

func checkReadableDir(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return errors.New("not a directory")
	}

	entry, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = entry.Close() }()

	// An empty directory returns io.EOF; only access failures are invalid.
	if _, err := entry.ReadDir(1); err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	return nil
}

func joinProblems(heading string, problems []error) error {
	listed := make([]error, 0, len(problems))
	for _, problem := range problems {
		listed = append(listed, fmt.Errorf("  - %w", problem))
	}
	return fmt.Errorf("%s\n%w", heading, errors.Join(listed...))
}

func valueOr(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
