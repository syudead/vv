# Technical debt

Track deliberate compromises that need follow-up. Each entry should identify
the affected area, impact, intended resolution, and an owner or trigger for
reconsideration.

## 記録

### TD-001: trigram では2文字以下の検索語に `MATCH` が一致しない

- 影響範囲: 検索（`internal/store/search.go`）
- 内容: FTS5 の trigram トークナイザは3文字単位で索引を作るため、`旅行` のような
  2文字の検索語は `MATCH` で一致しない。日本語では2文字の検索語が多い。
- 当面の対処: 3文字以上は `MATCH`、1〜2文字は FTS5 表への `LIKE '%…%'`（trigram 索引で
  処理される）に振り分ける。経路が2つになる分、検索の実装と試験が複雑になる。
- **実装済み（002）**: 振り分けは `internal/store/search.go` の `routeFor` にある。
  検索語は NFC 正規化してから文字数を数える（結合文字で書かれた「が」を2文字と
  数えると、2文字の入力が `MATCH` 経路へ回って0件になるため）。FTS5 の特殊文字は
  二重引用符で包んで無効化し、`LIKE` の `%`／`_` は `escape` 節で逃がす。
  並び順は一覧と同じ規則を使い、関連度（bm25）にはしない — `LIKE` 経路に関連度が
  無く、2つの経路で並びが変わると利用者から見て不可解になる。
  検証は `internal/store/fts_test.go`（2経路の振り分け、部分一致、1文字検索、
  正規化、特殊文字、並び順、検索とカーソルの併用）。
- 見直しの契機: 件数が増えて `LIKE` 経路の応答が実用的でなくなったとき。その時点で
  形態素解析ベースのトークナイザ（外部拡張）か、別の検索基盤を再検討する。
- 一次資料: [research.md R-001](../../specs/001-initial-setup/research.md)、
  [002 の R-110](../../specs/002-core-video-library/research.md)

### TD-003: コンテナのビルドではコミット情報が埋め込まれない

- 影響範囲: `/api/health` の応答（`commit`・`builtAt`)、`Dockerfile`
- 内容: [R-007] はビルド情報を `runtime/debug.ReadBuildInfo()` の
  `vcs.revision`／`vcs.time` から取る方針だが、これには `.git` とビルド環境の
  `git` が必要になる。`.dockerignore` で `.git` を除いているため、
  `task up`／`task build`（Docker 経路）では `-buildvcs=false` を指定しており、
  `commit` と `builtAt` は応答から省略される。手元の `task build` では埋め込まれる。
- 当面の対処: 契約（[contracts/openapi.yaml]）は `commit`・`builtAt` を
  「取得できない場合は省略される」任意項目としているため、応答としては適合している。
  リリース名は `VERSION` ビルド引数から `-ldflags` で渡している。
- 見直しの契機: 稼働中のイメージからコミットを特定したくなったとき。`.git` を
  context に含めて builder 段に `git` を入れるか、`COMMIT` ビルド引数を足す。
- 一次資料: [research.md R-007](../../specs/001-initial-setup/research.md)

### TD-004: Web の自動テストはビルド検証のみ（解消済み / 004）

- 状態: **解消済み**（004「現在の機能を前提とした UI の実装」）
- 影響範囲: `web/`（`task test-web`）
- 内容: Phase 0 の Web は画面が 1 つで、検証は `tsc --noEmit` と `vite build` が
  通ることまでとした（[plan.md] Technical Context の方針どおり）。単体テストの
  実行基盤（Vitest 等）は入れていないため、`App.tsx` の描画や `/api/health` の
  取得処理には自動検証がない。
- 当面の対処: 振る舞いの検証は Go 側（`internal/httpapi`）の経路テストで担保する。
- 見直しの契機: Phase 1 で一覧と詳細の画面が増えた時点。再生の E2E（Playwright）は
  Phase 3 の範囲。
- **契機に到達した（002）**: 画面が 1 つから 3 つ（一覧・再生・共通部品）に増え、
  自動検証の無いコードが `web/src/` に約 800 行ある。特に検証が薄いのは
  無限スクロールの継ぎ目（`useVideos` のカーソル引き継ぎ）、検索入力の待ち合わせと
  打ち切り、再生位置の送信（5 秒間隔・`visibilitychange` での `keepalive` fetch）で、
  いずれも Go 側の経路テストでは代替できない。
  それでも 002 では入れていない。判断は「画面が増えた時点で入れる」ことではなく
  「入れるなら実行基盤（Vitest + Testing Library）と、DOM を伴う検証の書き方を
  同時に決める」ことであり、002 の範囲（取り込み・一覧・再生・検索）と混ぜると
  どちらも中途半端になるためである。次の機能の着手時に、上の3点を最初の対象として
  導入する。
- **解消した変更（004）**: 実行基盤として Vitest + Testing Library（環境は `jsdom`）を
  導入し（004 の R-406）、`web/vite.config.ts` の `test` と `web/vitest.setup.ts` で
  設定した。`web/package.json` の `scripts.test` はビルド検証と `vitest run` の両方を
  走らせるので、`task test-web`（`task check` から呼ばれる）で両方が回る。
  名指しされていた 3 点は、書き換えの**前**に既存の実装に対して書き、書き換えの
  あとも同じ内容で通ることを確かめた（004 の FR-025 / SC-009）。

  | 名指しされていた点 | テスト |
  | --- | --- |
  | 無限スクロールの継ぎ目（カーソル引き継ぎ・打ち切り） | `web/src/api/useVideos.test.ts` |
  | 検索入力の待ち合わせと打ち切り（250ms） | `web/src/pages/LibraryPage.search.test.tsx` |
  | 再生位置の送信（5 秒間隔・離脱時の `keepalive` fetch） | `web/src/pages/VideoPage.progress.test.tsx` |

  3 点目を書いた時点で TD-008（離脱時に最後の位置が送られない）を見つけている。
  004 は既存の振る舞いを変えないことが要求なので、そちらは直さず書き留めてある。
- 一次資料: [plan.md](../../specs/001-initial-setup/plan.md)、
  [002 の plan.md](../../specs/002-core-video-library/plan.md)、
  [004 の R-406](../../specs/004-library-ui/research.md)

### TD-005: Go の依存が計画より1つ多い（`github.com/oapi-codegen/runtime`）

- 影響範囲: `go.mod`（`internal/httpapi/gen/api.gen.go` が import する）
- 内容: [002 の plan.md] は「新規に増やすのは `golang.org/x/text` と `react-router`
  の 2 つだけ」としていたが、実際にはもう1つ増えた。`api/openapi.yaml` に経路
  パラメータ（`/api/videos/{id}`）と問い合わせパラメータ（`limit`・`sort`・`cursor`）
  を足したことで、`oapi-codegen` の生成物が値の取り出しに
  `github.com/oapi-codegen/runtime` を使うようになったためである。
- 当面の対処: そのまま追加した。生成物は手編集しない方針（`AGENTS.md`）なので、
  この import を避けるには生成器を変えるか、パラメータの取り出しを手書きに戻す
  ことになり、どちらも「契約から生成する」という決定そのものを崩す。
  依存は生成器と同じ供給元で、版は `go.mod` に固定されている。
- 見直しの契機: `oapi-codegen` を差し替えるとき、あるいはこの依存が
  生成物以外から参照され始めたとき（その時点で境界が崩れている）。
- 一次資料: [002 の plan.md] の Technical Context / Primary Dependencies

### TD-006: quickstart S5 の期待値が、短い検証用動画では成立しない

- 影響範囲: [quickstart.md](../../specs/002-core-video-library/quickstart.md) の S5
- 内容: 視聴済みの判定は「残り 15 秒以内 **または** 95% 以上」である
  （[R-111]、`internal/domain/progress.go`）。S0 が作る検証用の動画は 8 秒なので、
  「残り 15 秒以内」が最初から成立する。S5 が期待する
  「`positionMs: 4000` で `completed: false`」は、この動画では得られない
  （`completed: true` になる）。規則そのものは正しく、検証手順の前提と
  噛み合っていない。
- 当面の対処: 受け入れ検証では S5 用に長め（2 分以上）の動画を使う。規則は
  research.md R-111とdata-model.mdが一致して定めているので、実装は規則どおりにした。
- 見直しの契機: quickstart を次に更新するとき。S0 に長めの動画を1本足し、
  S5 をそれに向けるのが素直である。
- 一次資料: [R-111]

### TD-007: 解消済み - Routine使用量ゲート

- 解消日: 2026-09-19
- 解消方法: Issue handoffへの移行で無人Routineと使用量ゲートを廃止した。専用status-line hookと
  一時JSONも削除し、各工程は保守者が明示的に開始する。

### TD-008: 再生画面を離れるとき、最後の位置が送られていない

- 影響範囲: 再生画面（`web/src/pages/VideoPage.tsx` の離脱時の送信）
- 内容: 離脱時の後片付けは `videoRef.current` を読んで `beaconProgress` を呼ぶが、
  React は**参照を外してから** `useEffect` の後片付けを呼ぶため、この経路では
  `videoRef.current` が必ず `null` になっている。したがって「一覧へ戻る」で
  画面を離れたときの最後の位置は送られず、直前の 5 秒周期の送信まで巻き戻る。
  タブを隠したとき（`visibilitychange`）の経路は参照が生きているので動いている。
  002 から入っていた欠落で、004 の Phase 2 で回帰の網を張ったときに見つかった。
- 影響の大きさ: 失うのは最大 5 秒ぶんの視聴位置である（[R-111] の送信間隔）。
  視聴済みの判定はサーバー側なので、見終わった動画の扱いは変わらない。
- 当面の対処: 振る舞いを変えずに、いまの状態を
  `web/src/pages/VideoPage.progress.test.tsx` に書き留めた。004 は既存の振る舞いを
  変えないことが要求（FR-025 / SC-009）なので、Phase 2 では直していない。
- 見直しの契機: **004と005のUI改修でも直していない**。005では再生位置の送信と
  再開の下限、および既存検査の意味を変えない方針だったためで、直すと上記の
  検査を書き換えることになり FR-018（004 の受け入れ基準を引き続き満たす）の
  確認が緩む。**次の契機は、再生位置の送信そのものを主題にする機能である。**
  最後の位置を参照ではなく状態として持ち回る形にすれば、後片付けの時点でも読める。
  直したら上記の検査の期待を「送られる」へ入れ替える。
- 一次資料: [R-111]、`web/src/pages/VideoPage.progress.test.tsx`

### TD-009: 004 の受け入れ検証のうち、実機が要る範囲が未実施

- 影響範囲: [004 の quickstart](../../specs/004-library-ui/quickstart.md) S0・S3・S5・S6・
  S7（サーバー側）・S9
- 内容: 004 の実装はセッション環境（Claude Code on the web の Linux コンテナ）で
  完了したが、そこには `ffmpeg`／`ffprobe` が無く、Docker のデーモンも動いていない。
  S0（検証用の動画をつくる）が実行できないため、それを前提にする実機の確認
  ── 見た目の判断、SC-001（5 人に「探す・並べ替える・取り込む」を尋ねる）、読み上げ
  ソフトでの確認、表示設定の実機確認、1 万本の実データでの SC-002、002 の S1〜S10 の
  再実行 ── はいずれも**していない**。
- 当面の対処: 画面側だけを本物にして測れる範囲を測った。`web` のビルドを
  `api/openapi.yaml` と同じ形を返す見本サーバーに載せ、Chromium（Playwright）で
  S4（4 幅の横スクロール・当たり判定・列数）、S7 の画面側（SC-002 は 240ms、
  SC-008 は 1020 件読み込み後で最悪 23.0ms）、S8 の一部（情報欄の言い分け・再生前の
  警告・途中からの再開・「動きを減らす」）、S10（画面 4 枚）を実行した。結果と
  範囲は [完了した実行計画](completed/004-library-ui.md)「受け入れ検証の結果」に
  ある。**サーバー側（SQLite の問い合わせ・`ffprobe`・取り込み）はこの検証に
  入っていない。**
- 見直しの契機: 保守者が `ffmpeg` と Docker のある環境で quickstart を通すとき。
  あるいは受け入れ検証を CI で回せる形（実データを作る手順の自動化）にするとき。
- 一次資料: [004 の quickstart.md](../../specs/004-library-ui/quickstart.md)、
  [完了した実行計画](completed/004-library-ui.md)

### TD-010: 再生画面で最初の `Tab` が × ではなく `<video>` に止まる

- 影響範囲: `web/src/pages/VideoPage.tsx` の 2 分割の器、
  [005 の quickstart](../../specs/005-ui-refinement/quickstart.md) S7-5
- 内容: [005 の R-509](../../specs/005-ui-refinement/research.md) は「SC-005 は再生画面側の
  ×（C15）が**最初の到達先である**ことで満たす」と書いている。実際の到達順は
  `<video>` → × で、× は 2 つめである。DOM が映像 → パネルの順だからで、R-509 の
  もう一方の決定（「DOM の順序をそのまま到達順にする。`tabIndex` で順序を操作
  しない」）には従っている。**同じ節の 2 つの文が食い違っている**のが実体である。
- 影響の大きさ: **SC-005 は成立している** — マウス 0 回で一覧 → 再生 → 一覧を往復
  できる（`Tab` が 1 回増えるだけで、行ける場所は変わらない）。`<video>` は
  それ自体が操作対象（再生・音量・全画面）なので、空振りの停止位置でもない。
- 当面の対処: 直していない。パネルを DOM で先にすれば × が最初になるが、幅 640 以上で
  見た目が「右のパネル → 左の映像」の順に読まれることになる。005 の Phase 7 は
  `flex-row-reverse` で見た目だけを入れ替える案を、読み上げの順序と目で見る順序が
  食い違うという理由で退けており、DOM 側から同じ食い違いを作り直すことになる。
  implement の回に `spec.md` と `research.md` は書き換えない。
- 見直しの契機: 再生画面のキーボード操作を主題にする機能、または読み上げでの確認
  （TD-011）で実害が出たとき。どちらを正とするかを先に決める —— × を最初にするなら
  R-509 の「DOM の順序をそのまま」を外す判断が要り、いまの順序を正とするなら
  R-509 の一文と quickstart S7-5 の期待値を直す。**quickstart の期待値だけを実装に
  合わせて書き換えるのは、いちばんやってはいけない直し方である。**
- 一次資料: [005 の R-509](../../specs/005-ui-refinement/research.md)、
  [005 の quickstart.md](../../specs/005-ui-refinement/quickstart.md) S7、
  [完了した実行計画](completed/005-ui-refinement.md)「決定の記録（Phase 8）」

### TD-011: 005 の受け入れ検証のうち、人と実データが要る 3 項目が未実施

- 影響範囲: [005 の quickstart](../../specs/005-ui-refinement/quickstart.md) S7-7・S7-8、
  [004 の quickstart](../../specs/004-library-ui/quickstart.md) S3（聞き取り）・S7（継ぎ目）
- 内容: 005 の実装はセッション環境（Claude Code on the web の Linux コンテナ）で完了した。
  S2〜S6・S8 の大半は `ffmpeg` を入れ、`task build` した実体を Chromium（Playwright）で
  操作して確かめられたが、次の 3 つは**していない**。
  1. **読み上げでの確認**（S7-7・S7-8）: 読み上げソフトが無い。淡い要素に `（未実装）` の
     補足語が付いていることと、`display: none` のロゴが支援技術からも消えることは DOM で
     確かめたが、**実際に読まれる音は聞いていない**。
  2. **初見の人への聞き取り**（004 の S3）: 「探す・並べ替える・取り込む」を 10 秒以内に
     指せるか。人が要る。帯そのものがヘッダーの下へ移り、周りに淡い要素が 15 個増えた
     ので、005 で再実施が要ると quickstart S8 が定めている。
  3. **無限スクロールの継ぎ目**（004 の S7）: 蔵書を 52 件までしか作れておらず、1 ページ
     （60 件）に収まるため継ぎ目が現れない。操作への反応が 69ms で始まることは測った。
- 影響の大きさ: 1 と 2 はアクセシビリティと初見の分かりやすさ、3 は大量件数での性能で、
  いずれも**機械の検査が代わりを務められない**範囲である。DOM が正しいことと、読まれる
  音・人の理解・1000 件での体感が正しいことは別の事実である。
- 当面の対処: 実施できなかったことを、成立した項目と区別して
  [完了した実行計画](completed/005-ui-refinement.md)「Phase 8 の検査の結果」に記録した。
  「DOM が正しいので成立」とは書いていない。
- 見直しの契機: 保守者が読み上げソフトのある環境で quickstart S7 を通すとき。3 は
  1000 件規模の蔵書（実データでも `ffmpeg` の量産でも可）を用意できるとき。
- 一次資料: [005 の quickstart.md](../../specs/005-ui-refinement/quickstart.md)、
  [完了した実行計画](completed/005-ui-refinement.md)

[R-111]: ../../specs/002-core-video-library/research.md
[002 の plan.md]: ../../specs/002-core-video-library/plan.md
[R-007]: ../../specs/001-initial-setup/research.md
[R-008]: ../../specs/001-initial-setup/research.md
[contracts/openapi.yaml]: ../../specs/001-initial-setup/contracts/openapi.yaml
[plan.md]: ../../specs/001-initial-setup/plan.md
### TD-010: 解消済み - Routineのbase branch filter

- 解消日: 2026-09-19
- 解消方法: Routineとmerge-triggered stage起動を廃止した。外部Routineの削除だけは保守者が
  claude.ai上で行う。

### TD-013: 解消済み - merge方式で変わるhop集計

- 解消日: 2026-09-19
- 解消方法: hop/retry集計とbranch名復元を廃止した。現在の工程判定は明示されたfeature
  directoryのartifactとGit祖先関係だけを使う。

### TD-014: Windows の `task dev` は孫プロセスを残すことがある

- 影響範囲: `scripts/dev`（`task dev`）
- 内容: 開発サーバーは2つとも孫プロセスを持つ（`go run` が起動するバイナリ、
  `npm` が起動する vite）。unix ではプロセスグループを分けてグループごと
  止めているが、Windows には同等の仕組みが無いため、起動したプロセスだけを
  止めている。孫が残ると vite が 5173 を掴んだままになり、次の `task dev` が
  `--strictPort` で失敗する。
- 当面の対処: 停止を待つのは `drainTimeout`（20 秒）までにして、孫が出力の口を
  握ったままでも `task dev` 自体は終わるようにした。これが無いと `Wait` が
  返らず端末が固まる。残ったプロセスは手で止める。PowerShell 版の `Stop-Job` は
  孫を待たずに戻っていたので、待たせない点だけを合わせている。
- 見直しの契機: Windows で実際に困ったとき。Job Object を使うか、
  `taskkill /T` 相当の停止に切り替える。
- 一次資料: `scripts/dev/procgroup_other.go`
