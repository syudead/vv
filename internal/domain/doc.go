// Package domain はドメインモデル（値の型と、外部 I/O に依存しない純粋な規則）を
// 保持する。ユースケースは internal/app に置く。
//
// このパッケージは依存の終端であり、他の internal パッケージからは参照されるだけで、
// 自分からは何も参照しない。外部 I/O にも依存しないため、net/http・database/sql・
// os/exec・modernc.org/sqlite・他の internal/* の import は禁止されている。
// この禁止は .golangci.yml の depguard で機械的に強制される。
//
// 依存の向き:
//
//	cmd → internal/{app,httpapi,store,media,artifacts,opener,scanner,jobs,eventbus} → internal/domain
//
// 詳細は ARCHITECTURE.md の "Intended dependency direction" を参照。
package domain
