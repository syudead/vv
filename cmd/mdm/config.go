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

// 設定は環境変数のみで与える。設定ファイルは持たない。
// 項目と既定値は README.md の「起動設定」にある。
const (
	envAddr     = "MDM_ADDR"
	envDataDir  = "MDM_DATA_DIR"
	envLogLevel = "MDM_LOG_LEVEL"
	// envTrustedProxies は転送ヘッダーを信じてよいリバースプロキシのアドレスの一覧である
	// （specs/016-single-account-auth/plan.md Structural Decisions 7）。未設定なら
	// ループバックとプライベートアドレス（defaultTrustedProxies）を信じ、none なら
	// 転送ヘッダーを読まない。
	envTrustedProxies = "MDM_TRUSTED_PROXIES"
)

// 既定値。すべて未設定でも起動できる。
const (
	defaultAddr     = ":8080"
	defaultDataDir  = "/data"
	defaultLogLevel = "info"
)

// defaultTrustedProxies は MDM_TRUSTED_PROXIES が未設定のときに信じる接続元である。
// 家庭の LAN や Docker のネットワークにある逆プロキシを、設定なしで使えるようにする。
// 同じ LAN の機器は送信元を偽れるが、インターネットからの要求はプロキシが付けた
// アドレスで数える。
var defaultTrustedProxies = []netip.Prefix{
	netip.MustParsePrefix("127.0.0.0/8"),
	netip.MustParsePrefix("10.0.0.0/8"),
	netip.MustParsePrefix("172.16.0.0/12"),
	netip.MustParsePrefix("192.168.0.0/16"),
	netip.MustParsePrefix("::1/128"),
	netip.MustParsePrefix("fc00::/7"),
}

// noTrustedProxies は、転送ヘッダーを読まないことを表す MDM_TRUSTED_PROXIES の値である。
const noTrustedProxies = "none"

// dataDirPerm は MDM_DATA_DIR とその配下を作成するときの許可属性である。
const dataDirPerm os.FileMode = 0o755

// thumbnailsDirName はサムネイルの置き場所である。MDM_DATA_DIR から導出し、
// 設定項目にはしない。
const thumbnailsDirName = "thumbnails"

// Config は起動時に組み立てる不変の設定である。
// データベースのパスは DataDir/mdm.db に固定し、設定項目にしない。
type Config struct {
	Addr     string
	DataDir  string
	LogLevel string
	// TrustedProxies は MDM_TRUSTED_PROXIES を解釈したもの。空なら転送ヘッダーを読まない。
	// 未設定なら defaultTrustedProxies になる。
	TrustedProxies []netip.Prefix
}

// logLevels は受け付ける記録の詳細度である。
var logLevels = map[string]slog.Level{
	"debug": slog.LevelDebug,
	"info":  slog.LevelInfo,
	"warn":  slog.LevelWarn,
	"error": slog.LevelError,
}

// LoadConfig は環境変数から設定を組み立て、値の形を検証する。
// ファイルシステムに触れる検証は Verify が行う。
//
// 不正な項目は**まとめて**列挙して返す。1つ見つけて即終了すると、設定を1回直すごとに
// 再起動する往復が生じるため。
func LoadConfig(getenv func(string) string) (Config, error) {
	cfg := Config{
		Addr:     valueOr(getenv(envAddr), defaultAddr),
		DataDir:  valueOr(getenv(envDataDir), defaultDataDir),
		LogLevel: valueOr(getenv(envLogLevel), defaultLogLevel),
	}

	var problems []error

	if _, _, err := net.SplitHostPort(cfg.Addr); err != nil {
		problems = append(problems, fmt.Errorf(
			"%s=%q は待ち受けアドレスとして解釈できません（例: :8080、127.0.0.1:8080）: %w",
			envAddr, cfg.Addr, err))
	}

	if !filepath.IsAbs(cfg.DataDir) {
		problems = append(problems, fmt.Errorf(
			"%s=%q は絶対パスではありません", envDataDir, cfg.DataDir))
	}

	if _, ok := logLevels[cfg.LogLevel]; !ok {
		problems = append(problems, fmt.Errorf(
			"%s=%q は未知の値です（debug / info / warn / error のいずれか）",
			envLogLevel, cfg.LogLevel))
	}

	proxies, proxyProblems := parseTrustedProxies(getenv(envTrustedProxies))
	cfg.TrustedProxies = proxies
	problems = append(problems, proxyProblems...)

	if len(problems) > 0 {
		return cfg, joinProblems("設定が正しくありません", problems)
	}
	return cfg, nil
}

// Verify はファイルシステム側の前提を確認する。
// DataDir は存在しなければ作成する。
// 不正な項目はここでもまとめて列挙する。
func (c Config) Verify() error {
	problems := c.verifyProblems()
	if len(problems) > 0 {
		return joinProblems("設定の検証に失敗しました", problems)
	}
	return nil
}

// verifyProblems は Verify が見つけた不備を列挙する。起動時は外部コマンドの
// 確認結果とまとめて1度に提示するため、誤りをまとめる前の形で返す。
func (c Config) verifyProblems() []error {
	var problems []error

	if err := os.MkdirAll(c.DataDir, dataDirPerm); err != nil {
		problems = append(problems, fmt.Errorf("%s=%s を作成できません: %w", envDataDir, c.DataDir, err))
	} else if err := checkReadableDir(c.DataDir); err != nil {
		problems = append(problems, fmt.Errorf("%s=%s を読み取れません: %w", envDataDir, c.DataDir, err))
	}

	// サムネイルの置き場所は起動後に初めて使うが、確認はここで済ませる。
	// 取り込みの途中で初めて失敗すると、一覧に画像が出ない理由が記録を
	// 追わないと分からなくなる。
	if err := os.MkdirAll(c.ThumbnailsDir(), dataDirPerm); err != nil {
		problems = append(problems, fmt.Errorf(
			"サムネイルの置き場所 %s を作成できません: %w", c.ThumbnailsDir(), err))
	}

	return problems
}

// Level は設定された詳細度を slog の水準に変換する。
// LoadConfig を通った値であれば必ず既知の値になっている。
func (c Config) Level() slog.Level {
	if level, ok := logLevels[c.LogLevel]; ok {
		return level
	}
	return slog.LevelInfo
}

// LogAttrs は有効な設定値を記録に出すための属性を返す。
func (c Config) LogAttrs() []slog.Attr {
	return []slog.Attr{
		slog.String(envAddr, c.Addr),
		slog.String(envDataDir, c.DataDir),
		slog.String(envLogLevel, c.LogLevel),
		slog.String(envTrustedProxies, formatPrefixes(c.TrustedProxies)),
	}
}

// parseTrustedProxies は MDM_TRUSTED_PROXIES を読む。値はカンマか空白で区切った CIDR
// （例: 127.0.0.1/32,10.0.0.0/8）で、1つのアドレスだけの項はその1つだけを表す。
// 空なら defaultTrustedProxies、none なら空の集合を返す。
// 解釈できない項はすべて誤りとして返す。
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
				"%s の %q は CIDR として解釈できません（例: 127.0.0.1/32、10.0.0.0/8、::1/128。転送ヘッダーを読まないなら %s）",
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
		return netip.Prefix{}, errors.New("CIDR ではありません")
	}
	return normalizePrefix(netip.PrefixFrom(addr, addr.BitLen())), nil
}

// normalizePrefix はホスト部を落とし、IPv4 射影の IPv6（::ffff:a.b.c.d/n）を IPv4 に直す。
// 送信元のアドレスは IPv4 に直してから比べるため、射影のままでは一致しない。
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

// ThumbnailsDir はサムネイルの置き場所を返す（MDM_DATA_DIR/thumbnails）。
// 設定項目にしないのは、置き場所が散らばるとバックアップと削除の手順が
// 増えるためである。
func (c Config) ThumbnailsDir() string {
	return filepath.Join(c.DataDir, thumbnailsDirName)
}

// checkReadableDir はディレクトリとして開けるかどうかを確認する。
func checkReadableDir(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return errors.New("ディレクトリではありません")
	}

	entry, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = entry.Close() }()

	// 空のディレクトリでは io.EOF が返る。読み取り権限が無い場合だけを不備とする。
	if _, err := entry.ReadDir(1); err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	return nil
}

// joinProblems は複数の不備を1つの誤りにまとめる。errors.Join の出力は改行区切りなので、
// 先頭に見出しを付けて箇条書きとして読める形にする。
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
