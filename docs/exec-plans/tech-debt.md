# Technical debt

Track deliberate compromises that need follow-up. Each entry should identify
the affected area, impact, intended resolution, and an owner or trigger for
reconsideration.

## 記録

### TD-001: trigram では2文字以下の検索語に `MATCH` が一致しない

- 影響範囲: 検索（`internal/store`、Phase 2 で実装）
- 内容: FTS5 の trigram トークナイザは3文字単位で索引を作るため、`旅行` のような
  2文字の検索語は `MATCH` で一致しない。日本語では2文字の検索語が多い。
- 当面の対処: 3文字以上は `MATCH`、1〜2文字は FTS5 表への `LIKE '%…%'`（trigram 索引で
  処理される）に振り分ける。経路が2つになる分、検索の実装と試験が複雑になる。
- 見直しの契機: 件数が増えて `LIKE` 経路の応答が実用的でなくなったとき。その時点で
  形態素解析ベースのトークナイザ（外部拡張）か、別の検索基盤を再検討する。
- 一次資料: [research.md R-001](../../specs/001-initial-setup/research.md)

### TD-002: `make build` が版管理している `web/dist/index.html` を上書きする

- 影響範囲: 開発者の作業環境（`web/dist`、`.gitignore`）
- 内容: `go:embed` は対象ディレクトリが存在しないとコンパイルが通らないため、
  `web/dist/.gitkeep` と最小の `index.html` を版管理している（[R-008]）。
  一方 Vite の出力先も `web/dist` なので、`make build` を実行すると版管理している
  プレースホルダが実際のビルド成果物で上書きされ、作業ツリーが汚れる。
- 当面の対処: `web/dist` 配下はこの 2 ファイル以外を `.gitignore` で除外し、
  ビルド検証（`make test-web`）は別の出力先（`web/.vite-build-check`）を使うことで
  `make check` が作業ツリーを汚さないようにした。`make build` 後の
  `web/dist/index.html` の差分はコミットしない。
- 見直しの契機: Vite の出力先を `web/dist` から動かせるようになったとき、あるいは
  埋め込み用のプレースホルダを版管理しなくても済む仕組みに変えたとき。
- 一次資料: [research.md R-008](../../specs/001-initial-setup/research.md)

### TD-003: コンテナのビルドではコミット情報が埋め込まれない

- 影響範囲: `/api/health` の応答（`commit`・`builtAt`)、`Dockerfile`
- 内容: [R-007] はビルド情報を `runtime/debug.ReadBuildInfo()` の
  `vcs.revision`／`vcs.time` から取る方針だが、これには `.git` とビルド環境の
  `git` が必要になる。`.dockerignore` で `.git` を除いているため、
  `make up`／`make build`（Docker 経路）では `-buildvcs=false` を指定しており、
  `commit` と `builtAt` は応答から省略される。手元の `make build` では埋め込まれる。
- 当面の対処: 契約（[contracts/openapi.yaml]）は `commit`・`builtAt` を
  「取得できない場合は省略される」任意項目としているため、応答としては適合している。
  リリース名は `VERSION` ビルド引数から `-ldflags` で渡している。
- 見直しの契機: 稼働中のイメージからコミットを特定したくなったとき。`.git` を
  context に含めて builder 段に `git` を入れるか、`COMMIT` ビルド引数を足す。
- 一次資料: [research.md R-007](../../specs/001-initial-setup/research.md)

### TD-004: Web の自動テストはビルド検証のみ

- 影響範囲: `web/`（`make test-web`）
- 内容: Phase 0 の Web は画面が 1 つで、検証は `tsc --noEmit` と `vite build` が
  通ることまでとした（[plan.md] Technical Context の方針どおり）。単体テストの
  実行基盤（Vitest 等）は入れていないため、`App.tsx` の描画や `/api/health` の
  取得処理には自動検証がない。
- 当面の対処: 振る舞いの検証は Go 側（`internal/httpapi`）の経路テストで担保する。
- 見直しの契機: Phase 1 で一覧と詳細の画面が増えた時点。再生の E2E（Playwright）は
  Phase 3 の範囲。
- 一次資料: [plan.md](../../specs/001-initial-setup/plan.md)

[R-007]: ../../specs/001-initial-setup/research.md
[R-008]: ../../specs/001-initial-setup/research.md
[contracts/openapi.yaml]: ../../specs/001-initial-setup/contracts/openapi.yaml
[plan.md]: ../../specs/001-initial-setup/plan.md
