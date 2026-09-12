// Package domain はドメインモデルとユースケースを保持する。
//
// このパッケージは依存の終端であり、他の internal パッケージからは参照されるだけで、
// 自分からは何も参照しない。外部 I/O にも依存しないため、net/http・database/sql・
// os/exec・modernc.org/sqlite・他の internal/* の import は禁止されている。
// この禁止は .golangci.yml の depguard で機械的に強制される。
//
// 依存の向き:
//
//	cmd → internal/{httpapi,store,media,scanner,jobs} → internal/domain
//
// 詳細は ARCHITECTURE.md の "Intended dependency direction" を参照。
package domain
