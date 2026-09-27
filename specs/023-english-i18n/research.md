# Research: サービスの英語化と i18n の基盤

引き継ぐ技術選定は [技術選定](../../docs/design-docs/tech-stack-selection.md) と
[ARCHITECTURE.md](../../ARCHITECTURE.md) にある。ここには、この feature が足す決定だけを置く。

調べた事実（2026-09 時点の `main`）:

- 画面: `web/src` の非テストの 124 ファイルが日本語を含み、そのうち文言は約 575 行、残りは
  コメントである。i18n ライブラリも文言の集約も無い。`ja-JP` は `lib/format.ts`・
  `player/properties.ts`・`settings/ScanStatusSection.tsx`・`videoList/listSummary.ts`・
  `library/SelectionBar.tsx`・`folders/*` にあり、相対時刻は `formatRelative` の手書きで、件数は
  「件」「本」「個」を足すだけで単数・複数の区別が無い。video.js は独自言語 `vv-ja` を
  `addLanguage` で持つ。`index.html` は `lang="ja"`。
- 画面の API エラー: `web/src/api/client.ts` の `RequestFailed` は `status`・`code`・`message` を
  持ち、`errorMessage()` がサーバーの `message` をそのまま画面へ渡す箇所が tags・settings・auth・
  shell・一覧の読み込み失敗にある。本文が JSON でなければ日本語の「要求に失敗しました」を作る。
- サーバー: `golangci-lint` の `gosmopolitan`（漢字・ひらがな・カタカナ、テストとコメントを除く）で
  `cmd/` と `internal/` に 710 件の日本語の文字列がある（`internal/store` 274、`internal/httpapi`
  206、`cmd/mdm` 61、`internal/media` 51、`internal/domain` 35、`internal/artifacts` 33、
  `internal/scanner` 20、`internal/app` 16、ほか少数）。ログは `log/slog` で本文が日本語、属性の
  キーは英語である。
- `Error` は `code`（22 種の enum）と `message` だけで `additionalProperties: false`。
  `invalid_request` は 48 か所、`not_found` は 22 か所、`conflict` は 7 か所、`forbidden` は 3 か所で
  使われ、状況は `message` でしか区別できない。
- 自由文の失敗理由は `videos.probe_error`（`Video.probeError`、画面は `<pre>` でそのまま出す）、
  `scans.error`（`Scan.error`、画面はそのまま出す）、`jobs.last_error`（API に出ない）の 3 列で、
  どれにも機械可読な理由は無い。本文は ffprobe の stderr の 1 行目やファイルの絶対パスを含む。

## R-1: 画面文言の置き場は型付きの自前カタログにする

- **Decision**: `web/src/i18n/` に、英語の文言カタログ（`en`）と、その型 `Messages` を置く。
  文言は画面の領域ごとの入れ子のオブジェクトで、埋め込みを持つ文言は引数を取る関数にする
  （`selectedCount: (n: number) => …`）。コンポーネントはカタログを静的に import して使う。
  言語は起動時に決まる 1 つの定数で、今回は `en` だけを持つ。
- **Rationale**: 文言の鍵と引数が TypeScript の型で検査されるので、存在しない鍵・引数の渡し忘れは
  `task check` の型検査で落ちる。将来の言語は `Messages` を満たすオブジェクトとして足すので、
  訳し漏れも型検査で見つかる（要件 7）。言語の切り替えは対象外なので、React の context や
  provider を挟まない。
- **Alternatives considered**:
  - i18next / react-intl / Lingui: 鍵が文字列で型検査が弱いか、抽出のビルド手順や ICU の解析器が
    増える。1 言語だけを持つ今回、依存を増やす利点が無い。
  - React context で `t()` を配る: 実行中に言語を変えないので、全テストに provider を足す手間だけが
    増える。

## R-2: 書式と単数・複数はブラウザの `Intl` で扱う

- **Decision**: `web/src/i18n/` に、カタログと同じロケールを使う書式関数（数、日付、日時、
  相対時刻、単数・複数の選択）を置く。相対時刻は `Intl.RelativeTimeFormat`（今の区切り
  「たった今・分・時間・日・週・か月・年」を保つ）、単数・複数は `Intl.PluralRules` を使い、
  カタログの関数が `one` と `other` の形を選ぶ。`ja-JP` とロケール引数の無い
  `toLocale*String` は `i18n/` の外から無くす。
- **Rationale**: ロケールを 1 か所で渡すので、日本語ロケールの固定が残らない（要件 3）。どの
  ブラウザにも組み込まれていて依存が増えない。
- **Alternatives considered**: 英語の複数形を `n === 1 ? … : …` で各所に書く方法は、別の言語の
  複数規則（0・few・many）を表せず、要件 7 の将来の言語を扱えない。

## R-3: 訳し漏れは lint と型で検出する

- **Decision**:
  - 画面: `web/eslint.config.js` に `no-restricted-syntax` を足し、`web/src` の非テストで
    `i18n/` の外にある、日本語を含む文字列リテラル・テンプレート・JSX テキスト、JSX の文字の
    テキスト、`aria-label`・`title`・`placeholder`・`alt` への文字列リテラル、`"ja-JP"`、ロケールを
    渡さない `toLocaleString`・`toLocaleDateString`・`toLocaleTimeString` を報告する。
  - 既知のエラー: カタログのエラー表を `Record<ErrorCode, …>` と `Record<ErrorReason, …>`
    （`api/openapi.yaml` から生成した型）にするので、API に足したコードや理由の表示漏れは型検査で
    落ちる。失敗理由のコード（[data-model.md](data-model.md)）も同じにする。
  - カタログの外の英語の文言（型）: カタログが返す値を `i18n/` の branded 型 `UiText` にし、固定の文言を
    運ぶ値をすべて `UiText` にする。自前の部品の props と引数（トーストの本文、状態表示・空状態・
    ダイアログの見出しと説明、ボタンの名前など）、文言を持つ state（`tags/tagNameField.ts` の理由など）、
    文言を返す helper（`api/folderGrouping.ts` の表示文など）、エラー表示の戻り値が対象である。英語の
    リテラルはそこへ入れられず、型検査で落ちる。利用者のデータ（動画名、タグ名）を運ぶ値は `string` の
    ままにする。
  - カタログの外の英語の文言（画面）: `i18n/` に、カタログのすべての文に印を付けた疑似ロケールを置き、
    テストでだけ差し替えられるようにする。各領域の主な画面のテストは疑似ロケールで描画し、見える
    テキストと `aria-label`・`title`・`placeholder`・`alt` のすべてが、印を持つか、テストが与えた
    利用者のデータであることを確かめる helper を通す。`string` の変数を経由した英語のリテラルも、
    描画された画面で印が無いので落ちる。
  - サーバー: `.golangci.yml` で `gosmopolitan`（`watch-for-scripts: [Han, Hiragana, Katakana]`、
    `allow-time-local: true`）を `cmd/` と `internal/` の非テストに有効にする。
- **Rationale**: どれも AST を見るので、日本語のまま残すコメントを誤検出しない。ESLint の組み込み
  規則と golangci-lint の同梱 linter なので依存が増えない。`gosmopolitan` は今の `main` で
  上の 710 件をパッケージごとに報告することを確かめた。
- **Alternatives considered**:
  - `web/src/theme/tokens.test.ts` のような行単位の走査: 日本語のコメントを文言と区別できない。
  - Go の独自 AST テスト: 同梱の `gosmopolitan` で足りる。
  - 実行時に未知の鍵を警告する: 画面を開くまで見つからない。
  - 英語らしいリテラル（空白で区切られた英単語）を ESLint で報告する: 今の `web/src` でこの形の
    リテラルは `className` の属性を除いても 685 あり、ほぼすべてが helper や変数に置いた Tailwind の
    クラス列で、文言と区別できない。

  型は書いた時点で、疑似ロケールの画面テストは型をすり抜けた経路（`string` の変数を経由した表示）を、
  テストが描画する状態について捕まえる。テストが描画しない状態は、各領域の単位の受け入れ条件が
  挙げる状態（通常・空・処理中・失敗）を疑似ロケールのテストで描画することで覆う。

移行の途中で lint を通すため、画面の規則は基盤の単位で有効にし、まだ英語にしていない
ディレクトリを一時的な除外の一覧に置く。各領域の単位が自分のディレクトリを一覧から外し、
最後の単位で一覧が無くなる。`gosmopolitan` はサーバーの最後の単位で有効にする。

## R-4: API エラーの具体性は、コードを変えずに `reason` で足す

- **Decision**: `Error` に任意の `reason`（機械可読な下位の理由の enum）、`limit`（整数）、`tagName`
  （競合先のタグの元の名前）を足す。
  `reason` は、1 つのコードが画面から起こりうる複数の状況に使われている箇所にだけ付ける
  （[contracts/error-api.md §1](contracts/error-api.md#1-error-の-reasonlimittagname)）。画面の
  表示は `reason` → `code` の順にカタログを引く。
- **Rationale**: 要件 5 が既存の `code` と HTTP 状態を保つことを求めるので、同じ状況に新しい
  `code` を割り当てられない。今の `message` が埋め込む値のうち画面の説明に要るもの（上限値と競合先の
  タグ名）を項目で返すので、`message` を使わずに具体性を保てる。上限値を `limit` で返すので、画面がサーバーの上限（ユーザー名の長さ、
  タグ名の長さ、検索語の長さ、一括操作の件数）を二重に持たない。
- **Alternatives considered**:
  - 状況ごとに新しい `code` を足す: 今の `invalid_request` などを受け取っている API 利用者に対して
    `code` が変わり、要件 5 に反する。
  - `params` を任意の値の辞書にする: 今必要な値は上限とタグ名の 2 つだけで、型の無い辞書は生成型での
    検査が効かない。
  - 全 48 の `invalid_request` に理由を付ける: 画面が送らない値（並び順の名前、`attempt` の形式、
    Content-Type など）は API の誤用で、コードの英語文と `message` で足りる。

## R-5: 画面は、既知のエラーを `reason`・`code` から、それ以外を安全な概要で表示する

- **Decision**: `web/src/api/client.ts` の `RequestFailed` に `reason`・`limit`・`tagName` を足し、
  `errorMessage()` を `i18n/` のエラー表示に置き換える。
  - 既知の `reason` または `code`: カタログの英語文（`limit`・`tagName` を埋め込む）。
  - 未知の `code` で `message` がある: サーバーの英語の `message`（要件 6 のフォールバック）。
  - 本文が JSON でない・空・`message` が無い: `Request failed (HTTP <status>)` の形の英語の概要。
  - `fetch` 自体の失敗: サーバーに届かなかったことを表す英語の概要。ブラウザの文言は出さない。
  操作ごとに文脈が必要な表示（「タグ名として使えない」がフォルダ名かタグ名か）は、今と同じく
  呼び出し側の文言が囲む。
- **Rationale**: API の `message` は API 利用者向けの英語として残しつつ、画面の文言をカタログ
  1 か所で持てる。未知のコードでも `status` と `code` は `RequestFailed` に残るので、診断の手掛かりを
  失わない。
- **Alternatives considered**: 未知のコードでも `message` を捨てて一般的な概要だけを出す方法は、
  要件 6 が `message` を未知のコードの英語フォールバックとすることに反する。

## R-6: 失敗理由は機械可読なコードを保存し、画面は自由文を出さない

- **Decision**: 解析と取り込みの失敗に、自由文とは別に理由のコードを保存する
  （[data-model.md](data-model.md)）。コードは失敗を作る adapter（`internal/media`・
  `internal/scanner`）が `internal/domain` の失敗型で包み、`internal/app`・`internal/jobs`・
  `internal/store` は `errors.As` で取り出す。分類できない失敗は `internal` になる。画面は
  `probeError` と `Scan.error` の自由文を表示せず、コード（と取り込み失敗のメディアフォルダの
  パス）から英語の説明を作る。コードの無い行（アップグレード前の行）は一般的な英語の概要を出す。
- **Rationale**: 自由文には ffprobe の stderr や OS のエラー文が入り、アップグレード前の行は日本語
  なので、画面に出すと要件 8 と Edge Cases（外部プログラムの自由文を直接出さない、過去の
  日本語を漏らさない）に反する。コードを別の列に持つので、保存済みの自由文は書き換えずに残る。
- **Alternatives considered**:
  - 自由文を画面で解析して分類する: 文言を変えるたびに壊れ、過去の日本語も扱えない。
  - 保存済みの日本語を移行で英語に書き換える: 機械翻訳は対象外で、要件 8 は保存済みの理由を
    保持することを求める。
  - 自由文を「技術的な詳細」として畳んで出す: 過去の日本語と新しい英語を画面が見分けられない。

## R-7: サーバーには文言カタログを置かず、英語を直接書く

- **Decision**: API の `message`、ログ、`cmd/mdm` の設定エラー・アカウント操作の出力は、Go の
  コードに英語で直接書く。`internalError` がログと応答に同じ文を使う今の形は保つ。`/api/events`
  の購読者名など、利用者に出ない識別子も英語にする。
- **Rationale**: 要件 5 はサーバーの出力を英語に統一することだけを求め、言語ごとの表示は画面が
  コードから行う（R-5）。サーバーが `Accept-Language` で出し分けると、同じ文言を画面とサーバーの
  2 か所で持つことになる。
- **Alternatives considered**: Go 側にも文言カタログを置く: 使う言語が英語 1 つで、画面の表示に
  使わない文言を集約する利点が無い。

## R-8: 英語化しないもの

- **Decision**: コードのコメント、`api/openapi.yaml` の `description`、設計文書、テスト名は
  日本語のままにする。`scripts/`（`task doctor`・`task dev`・`previewbench` などの開発者向け
  道具）は変えない。和文フォント（Noto Sans JP）は残す。
- **Rationale**: 要件はサービスが生成する利用者向け・運用者向けの文言を対象にし、リポジトリの
  文書の言語は対象にしない。`scripts/` はリポジトリの開発環境で動く道具で、配布されるサービスの
  出力ではない。和文フォントは、日本語の動画名・フォルダ名・タグ名を英語の画面でも正しく表示する
  ために要る（Edge Cases）。
- **Alternatives considered**: `scripts/` も英語にする: 要件の対象外の範囲に広がる。
  和文フォントを外す: 日本語の名前が代替フォントで描かれ、見た目が崩れる。

## R-9: video.js の文言はカタログから作る

- **Decision**: `web/src/player/VideoPlayer.tsx` の独自言語 `vv-ja` を、カタログの英語から作る
  独自言語に置き換える。キーボード操作を持つボタンにキーを添える今の形（`Play (Space)` など）は
  保つ。
- **Rationale**: video.js 既定の英語はキーの案内を持たないので、独自言語を残す必要がある。文言の
  置き場をカタログに揃える。
- **Alternatives considered**: video.js 既定の `en` に任せる: キーボード操作の案内がツールチップと
  読み上げ名から消え、要件 2・4 に反する。
