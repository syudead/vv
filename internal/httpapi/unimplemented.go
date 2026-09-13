package httpapi

import (
	"net/http"

	"github.com/syudead/vv/internal/httpapi/gen"
)

// 契約（api/openapi.yaml）にはあるが、まだ実装していない経路をここに置く。
//
// 生成された gen.ServerInterface はすべての経路の実装を要求するので、
// 未実装のものがあるとコンパイルが通らない。その性質を残したまま段階的に
// 進めるため、未実装であることが応答からも分かる形で一時的に置いている。
// 実装が入った時点でこのファイルは消える。
//
// 対象: 再生（GET /api/videos/{id}/stream）と再生位置の記録
// （PUT /api/videos/{id}/progress）。どちらも
// specs/002-core-video-library/tasks.md の User Story 2 で入る。

// StreamVideo は未実装である。
func (s *server) StreamVideo(w http.ResponseWriter, _ *http.Request, _ gen.VideoId) {
	s.notImplemented(w, "再生はまだ利用できません")
}

// PutVideoProgress は未実装である。
func (s *server) PutVideoProgress(w http.ResponseWriter, _ *http.Request, _ gen.VideoId) {
	s.notImplemented(w, "再生位置の記録はまだ利用できません")
}

// notImplemented は「経路はあるが中身が無い」ことを返す。404 にすると綴りの
// 誤りと区別が付かないため、501 を使う。
func (s *server) notImplemented(w http.ResponseWriter, message string) {
	s.writeError(w, http.StatusNotImplemented, codeInternal, message)
}
