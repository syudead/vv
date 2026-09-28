# UI Design: 設定ページの「API トークン」節

**Feature**: [parent Issue #493](https://github.com/syudead/vv/issues/493) ・
[plan.md](plan.md) ・ [contracts/token-api.md](contracts/token-api.md) ・
[research.md R-1](research.md#r-1-トークンは接頭辞付きの-256-ビットの乱数にしsha-256-だけを保存する)・
[R-10](research.md#r-10-トークンの名前の規則はタグ名と同じ形にする)

見た目の規則は次の文書に従い、ここでは決め直さない。

- 配色・操作状態・幅の出し分け: [ライブラリ UI](../../docs/design-docs/library-ui.md)
- role token: [`web/src/index.css`](../../web/src/index.css) の `@theme`
- 対比を検査する組: [`web/src/theme/tokens.test.ts`](../../web/src/theme/tokens.test.ts)
- 設定ページの節の枠・見出し・一覧の行・確認の窓の形: 今の
  [`web/src/settings/SettingsPage.tsx`](../../web/src/settings/SettingsPage.tsx)（メディアフォルダ）と
  [`TranscodingSection.tsx`](../../web/src/settings/TranscodingSection.tsx)（動画の変換）
- 文言の置き場と書式: [画面の文言と書式（i18n）](../../docs/design-docs/i18n.md)。本書の英語は
  意図を示す案で、実装後はカタログ [`web/src/i18n/en.ts`](../../web/src/i18n/en.ts) が正本になる。
  カタログは 023 の決定どおり英語の 1 組で、本書は言語の仕組みを足さない（UI品質「日本語対応は求めない」）

この feature が画面に足すのは、設定ページの 1 つの節だけである。ほかの画面は変えない。ゲストは
設定ページに入れない（016 のゲート）ので、ゲスト向けの縮退は無い。`mdm account` の出力の変更は
CLI で、本書の範囲外である（[contracts/token-api.md](contracts/token-api.md#mdm-account)）。
新しい色・半径・影の token は足さない。`tokens.test.ts` の `pairs` には、この節で初めて
検査する組を 2 つだけ足す（下の「Colour」）。

## Screen boundary

- 節は設定ページの**最後**、「動画の変換」の下に置く。節の枠（`mt-8 rounded-lg border
  border-border bg-surface p-4 sm:p-5`）、見出し（`h2`、`text-base font-semibold`）、見出しの下の
  説明（`text-sm leading-6 text-fg-muted`、`max-w-2xl`）、その下の区切り線は、メディアフォルダと
  動画の変換の節と同じにする。最後に置くのは、オーナーが時々しか使わない管理機能で、
  取り込みの状態やフォルダより先に読ませる理由が無いからである（UI品質「既存の節より目立つ
  見た目にしない」）。
- 節の中は上から次の順である。
  1. 見出しの行と説明
  2. 発行の form、または発行直後の平文の表示（どちらか一方）
  3. トークンの一覧
- 発行の入口を一覧の上に置くのは、メディアフォルダ（「Add folder」が一覧の下）と違う。発行直後の
  平文は、それが作った行のすぐ近くで、節の中で最も目立つ位置に出す必要があり（UI品質
  「視覚的階層」）、その位置は見出しの直下である。form と平文の表示を同じ場所で入れ替えると、
  発行 → 平文 → 一覧の先頭の行、と目が上から下へ一度で通る。メディアフォルダの「Add folder」は
  窓を開くだけの button で、その位置は同じ意味を持たない。
- 節は取り込み（スキャン）の状態に影響されない。メディアフォルダの「スキャン中は変更できない」の
  無効化は、この節には無い。
- 失効の確認は、メディアフォルダの「フォルダの削除」と同じ `ModalFrame` の窓で行う（下の
  「Revoke」）。それ以外に窓・別ページ・popover は無い。

## Words

| 場所 | 英語（案） |
| --- | --- |
| 見出し | API tokens |
| 説明 | Tokens let external tools and MCP clients use this library as the owner. Anyone who has a token can read every video and change tags, so keep it as safe as your password. |
| 説明の末尾のリンク | How to use the API and MCP（`docs/how-to/external-api.md` の公開文書。動画の変換の節の「How to set up hardware encoding」と同じ形: `text-link underline`、`ExternalLink`、新しいタブ） |
| 名前の入力のラベル | Name |
| 名前の入力の補足 | What the token is for, such as the tool that will use it. |
| 発行の button | Create token |
| 発行中 | Creating… |
| 平文の表示の見出し | Token for 〈名前〉 |
| 平文の下の注意 | This is the only time the token is shown. Copy it now. After you close this, it can't be shown again; if you lose it, revoke it and create a new one. |
| コピーの button | Copy token |
| コピーした後の toast | Copied |
| コピーできなかった後の toast | Couldn't copy. Select the token and copy it yourself. |
| 平文を閉じる button | Done |
| 一覧の作成日時 | Created 〈日時〉 |
| 一覧の最終使用 | Last used 〈相対時刻〉／Never used |
| 失効の button | Revoke |
| 失効の窓の見出し | Revoke this token? |
| 失効の窓の対象のラベル | Token |
| 失効の窓の注意 | Tools using this token stop working right away. This can't be undone. |
| 失効の窓の button | Revoke／Revoking… |
| 失効した後の toast | Revoked |
| 読み込み中 | Loading the API tokens |
| 読み込みの失敗 | Couldn't load the API tokens: 〈理由〉 |
| 発行の失敗（`reason` 別） | `api_token_name_empty`: Enter a name for the token. ／ `api_token_name_control_characters`: The name can't contain control characters. ／ `api_token_name_too_long`: The name can be up to 〈`limit`〉 characters. |

- 日時は `formatDateTime`（例: Sep 28, 2026, 3:04 PM）。最終使用は `formatRelative`（例: 3
  minutes ago）で書き、`title` に `formatDateTime` の日時を持たせる。最終使用は「いつ頃まで
  使われていたか」を読むもので、分の精度しか無い（R-9）ので、相対の形が合う。作成日時は
  絶対の日時で、同じ名前のトークンを見分ける手掛かりになる（Edge Cases「同じ名前のトークンは
  複数作れる」）。
- 名前・平文は利用者のデータとして引数で埋め込み、訳さない。
- 節の中に「トークンが無い」ことを言う行は置かない。トークンが無いときは説明と発行の form だけが
  見え、それが「何のための節か」と「発行の入口」である（UI品質「情報密度」）。

## Create

発行の form は見出しの区切り線の下（`pt-4`）に置く。

- **入力**: ラベル「Name」（`text-xs font-medium text-fg-muted`、`label` の `htmlFor` で結ぶ）と、
  ログイン画面と同じ文字入力（016 ui-design「Typography」の `h-9 rounded-sm border border-border
  bg-field px-3 text-sm text-fg focus:border-accent focus:outline-none`）。`md` の `Button` と
  高さをそろえるためである。`autocomplete="off"`、`maxLength` は付けない（上限は R-10 の
  サーバーの規則で、超えたときの文言はサーバーの `reason` から作る）。入力の下に補足を
  `text-xs text-fg-muted` で 1 行。
- **主操作**: `Button` の primary「Create token」。名前の前後の空白を除いて空のあいだは
  `disabled`。発行中は spinner（`LoaderCircle`）と「Creating…」にし、入力も `disabled` にする。
  Enter で送る（`form` の submit）。
- **失敗の行**: 入力の下、補足の代わりに `text-sm text-danger`・`role="alert"` の 1 行。
  `reason` のある失敗は上の表の文、それ以外は `errorText` の文。入力の値は残す。
- **成功**: 201 の `token` を一覧の先頭に足し、form を平文の表示（下の「Reveal」）に置き換える。
  入力の値は空に戻す。toast は出さない。平文の表示自体が結果であり、toast は視線を逸らす。
- **一覧の読み込みが失敗しているあいだ**は、form を出したまま入力と button を `disabled` にし、
  button の下に「You can create a token once the current tokens have loaded」を `text-xs
  text-fg-muted` で出す（メディアフォルダの「Add folder」と同じ扱い）。

## Reveal

発行直後の平文の表示は、form と同じ場所に、form の代わりに出す。この feature で唯一、
節の中で他より強く見せる要素である。

- **面**: メディアフォルダの行の「Current location」と同じ箱（`rounded-md border
  border-control-border bg-field p-3 sm:p-4`）。位置（見出しの直下）、中の主操作、注意の行で
  目立たせ、面の色や枠の色は変えない。目立つ色の面を足すと、設定ページの他の節より節全体が
  騒がしくなる（UI品質「既存の節より目立つ見た目にしない」）。
- **中身**（上から）:
  1. 「Token for 〈名前〉」を `text-xs text-fg-muted` で。名前は `break-words`。
  2. 平文を `code`（`font-mono`、`text-sm text-fg`、`break-all`、`select-all`）で。46 文字
     （`vvt_` と 43 文字、R-1）なので、360px では 2〜3 行に折り返す。伏せ字にはしない。一度しか
     見られないものを、さらに「表示」の操作で隠さない。
  3. 操作の行: `Button` の primary「Copy token」（`Copy` のアイコン）と、secondary「Done」。
     Copy が先で、この節でこのとき唯一の primary である（操作の優先順位: 発行 ＞ コピー）。
  4. 注意の行: `text-sm leading-6 text-warning`、先頭に `ShieldAlert`（`size-4`、`mt-1`）、左に
     `border-l-2 border-warning-strong pl-3`（ログイン画面の接続の警告と同じ形）。文は上の表。
- **コピー**: 再生画面の「パスをコピー」と同じ作り（`navigator.clipboard`、安全でない接続では
  選択によるコピーへ切り替え）。成功で「Copied」、失敗で「Couldn't copy. …」の toast。コピーが
  失敗しても平文は残り、`select-all` で手で選べる。
- **閉じる**: 「Done」でだけ閉じ、form に戻る。Esc・外側の押下・時間で閉じない。
  ページの読み直しでも消え、二度と出ない（要件 2）。閉じるときの確認は挟まない。
  注意の行が「閉じると二度と出ない」と「失ったら失効して作り直す」を伝えており、コピーを
  忘れたときの回復（失効 → 発行）が安い。
- **窓にしない理由**: `ModalFrame` は Esc と外側の押下で閉じ、うっかり閉じると平文を失う。
  その場に置く形なら、閉じるのは「Done」だけである。
- 平文の表示が出ているあいだも一覧は操作できる。同じトークンを失効すると、平文の表示も
  一緒に消す（失効したトークンの平文を残さない）。

## List

- form（または平文の表示）の下、`mt-5`。行は `divide-y divide-border`、各行 `py-4`
  （メディアフォルダの行と同じ）。並びは API の順（作成の新しい順）で、画面で並べ替えない。
- **行**: 左に名前と日時のまとまり（`min-w-0 flex-1`）、右に「Revoke」（`shrink-0`、行の上端に
  そろえる）。
  - 名前: `text-sm font-medium text-fg`、`break-words`。省略しない。100 文字までで、
    続きが無いことを確かめられる方が、`title` で全体を読ませるより速い。
  - 日時の行: `mt-1 text-xs text-fg-muted tabular-nums`。「Created 〈日時〉」と
    「Last used 〈相対〉」（または「Never used」）を `flex flex-wrap gap-x-3` で並べ、収まらない幅では
    2 行になる。
  - 「Never used」は日時と同じ色で書き、強調しない。未使用は誤りではない。
- **失効の button**: `Button` の ghost・`sm`・`text-danger`、`Trash2` のアイコン、文言「Revoke」
  （メディアフォルダの「Remove」と同じ形）。行の失効中は spinner と「Revoking…」の代わりに
  `disabled` にし、窓の button に進行を出す（下）。
- **読み込み中**: メディアフォルダと同じ `Skeleton` を 2 本。
- **読み込みの失敗**: 「Couldn't load the API tokens: 〈理由〉」（`text-sm text-danger`）と
  `Button`「Retry」。form は上の「Create」のとおり無効のまま見せる。
- **空**: 一覧の領域を出さない（上の「Words」）。

## Revoke

- 「Revoke」を押すと `ModalFrame` の窓を開く。中身はメディアフォルダの削除の窓と同じ構成である。
  1. 対象の箱（`rounded-md border border-control-border bg-field p-3`）: ラベル「Token」
     （`text-xs text-fg-muted`）、名前（`text-sm text-fg`、`break-words`）、その下に
     「Created 〈日時〉」（`text-xs text-fg-muted`）。同じ名前のトークンを、作成日時で
     見分けるためである。
  2. 注意の行: `border-l-2 border-danger-strong pl-3 text-sm leading-6 text-fg-muted`。
  3. 失敗の行（あれば）: `role="alert"`、`text-sm text-danger`。
  4. 下段: secondary「Cancel」（初期 focus）と danger「Revoke」。実行中は danger の button が
     spinner と「Revoking…」になり、両方 `disabled`。
- 成功で窓を閉じ、行を一覧から消し、toast「Revoked」。focus は次の行の「Revoke」へ、行が
  無ければ名前の入力へ移す（メディアフォルダの削除後と同じ）。
- 失効の窓の初期 focus が「Cancel」なのは、Enter の連打で取り消せない操作を確定させない
  ためである（UI品質「失効は取り消せないため確認を挟む」）。

## Interaction states

- 操作状態（hover・focus-visible・active・disabled）は `Button`・文字入力の
  既存の状態を使い、この節で新しい状態を作らない。
- 発行中は form の入力と button、失効中は窓の 2 つの button が `disabled` になる。一覧の
  「Revoke」は発行中も押せる（発行と失効は別のトークンに対する独立の操作である）。
- 所有者として描いている間に 401 を受けたときは、016 のゲートの規則（ページを 1 度読み直す）に
  従う。読み直しで平文の表示は消える。

## Colour

新しい token は足さない。使う組のうち次は `tokens.test.ts` の `pairs` にすでにある: 本文
（`fg` on `surface`・`field`・`elevated`、`fg-muted` on `surface`・`elevated`）、失敗の行
（`danger` on `surface`・`elevated`）、箱の枠（`control-border` on `surface`・`field`・
`elevated`）、主操作（`accent-fg` on `accent`）。

平文の表示と失効の窓の対象の箱（`bg-field`）の上の文字は、次の 2 組がまだ `pairs` に無いので、
実装で `pairs` に足す（いずれも 4.5 以上）。

- `fg-muted` on `field`: 「Token for 〈名前〉」、失効の窓の「Token」と「Created 〈日時〉」。
- `warning` on `field`: 平文の表示の注意の行。

`border-warning-strong`・`border-danger-strong` は 016 と設定ページがすでに同じ用途で使っている
左の線で、文字ではなく文とアイコンが意味を運ぶので、組には足さない。

## Responsive layout

判定する幅は 360px・768px・1280px である。設定ページの `max-w-4xl` の中に収め、1280px でも
節を広げない。

- **360px**:
  - form: ラベル、入力（`w-full`）、補足、button（`w-full`）の縦並び。
  - 平文の表示: 平文は 2〜3 行に折り返し、箱の外にはみ出さない。操作の行は「Copy token」と
    「Done」を 1 行に並べ、収まらなければ折り返す。注意の行は左の線に沿って折り返す。
  - 一覧の行: 名前が折り返しても「Revoke」は右端の上端に留まる。日時の行は「Created …」と
    「Last used …」の 2 行になる。
  - 失効の窓: `ModalFrame` の既存の狭幅の形。
- **768px 以上**:
  - form: ラベルと補足はそのまま、入力（`max-w-sm`）と「Create token」を 1 行に並べる。
  - 一覧の行: 名前 1 行・日時 1 行の 2 行に収まる（100 文字の名前は 2 行になってよい）。
  - 平文は 1〜2 行。
- **1280px**: 768px と同じ。節の幅は他の節と同じで、平文の箱を節の幅いっぱいにする
  （`max-w-2xl` で止めない。平文は 1 行で読める方がコピーの確認がしやすい）。

## Review criteria

360px・768px・1280px の各幅で、次を目で見て判定する。UI品質の 5 観点（視覚的階層・情報密度・
余白のリズム・タイポグラフィ・操作の優先順位）に対応させてある。

1. **階層**: 節を開いたとき、見出し → 発行の入口 → 一覧の順に読める。発行直後は、平文の箱と
   「Copy token」が節の中で最も強く、見出しより先に目に入る。それ以外のときに節の中で他より
   強い要素が無い。
2. **一度きり**: 発行 → 「Done」→ 読み直し、の後、節のどこにも平文の一部（`vvt_` を含む文字列）が
   無く、一覧には名前と日時だけが残る。
3. **密度**: トークン 3 本の節の高さが、フォルダ 3 つのメディアフォルダの節と同じ程度である。
   1 本が 768px 以上で 2 行に収まり、名前が日時より強い。トークンが無いとき、節は見出し・説明・
   form だけで、「無い」ことを言う行も空の枠も無い。
4. **余白のリズム**: 節の上の間隔、枠、見出しの下の区切り線、行の区切りと上下の余白が、
   メディアフォルダの節と見分けがつかない。form と一覧の間、平文の箱の中の要素の間が、
   同じ節の他の間隔より詰まったり空いたりしていない。
5. **タイポグラフィ**: 平文だけが等幅で、それ以外は他の節と同じ書体と大きさである。360px で
   平文と 100 文字の空白無しの名前が箱や行からはみ出さない。日時の数字が行ごとにずれない
   （`tabular-nums`）。
6. **操作の優先順位**: 節に primary の button は常に 1 つだけ（「Create token」または
   「Copy token」）。「Revoke」は ghost の危険色で、primary より弱く、押すと必ず窓が出て、
   窓の初期 focus は「Cancel」にある。
7. **目立ちすぎない**: 設定ページ全体を見たとき、この節が取り込みの状態やメディアフォルダより
   目を引かない。アクセント色は primary の button と focus の輪郭以外に無い。
8. **重ならない**: 平文の表示と toast、失効の窓と toast が、どの幅でも重ならず読める。
