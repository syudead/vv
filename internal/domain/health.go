package domain

import "time"

// DefaultVersion はリリース名が指定されないままビルドされた場合の既定値である。
// ビルド時に -ldflags "-X main.version=…" で上書きできる。
const DefaultVersion = "dev"

// Status は稼働状態を表す。取り得る値は ok と degraded の2つだけで、
// 表現は api/openapi.yaml の Health.status に対応する。
type Status string

const (
	// StatusOK は保存層まで疎通できている状態。
	StatusOK Status = "ok"
	// StatusDegraded はプロセスは生きているが保存層へ疎通できない状態。
	StatusDegraded Status = "degraded"
)

// Valid は既知の状態かどうかを返す。
func (s Status) Valid() bool {
	switch s {
	case StatusOK, StatusDegraded:
		return true
	default:
		return false
	}
}

// BuildInfo は稼働中のバイナリを特定するための情報である。
// Commit と BuiltAt は取得できない場合に空（ゼロ値）になる。
type BuildInfo struct {
	Version string
	Commit  string
	BuiltAt time.Time
}

// Health は稼働情報の値である。状態は保持せず、要求のたびに組み立てる。
type Health struct {
	Status  Status
	Version string
	Commit  string
	BuiltAt time.Time
}

// NewHealth は保存層への疎通結果とビルド情報から稼働情報を組み立てる。
// 判定は呼び出しのたびに行い、結果をパッケージ内に保持しない。
func NewHealth(build BuildInfo, storeReachable bool) Health {
	status := StatusDegraded
	if storeReachable {
		status = StatusOK
	}

	version := build.Version
	if version == "" {
		version = DefaultVersion
	}

	return Health{
		Status:  status,
		Version: version,
		Commit:  build.Commit,
		BuiltAt: build.BuiltAt,
	}
}
