# UI の変更を画像で示す

画面の見た目や操作が変わる変更は、文章だけでは差分が伝わらない。PR を読む側が
チェックアウトして起動しなくても判断できるよう、変更後の画面を画像で添える。

## いつ必要か

- `web/` 配下の描画・配置・文言・操作の流れが変わる変更
- サーバー側の変更でも、画面の表示が変わるもの（表示項目の増減、エラー表示など）

画面に変化がない変更（内部実装、テスト、文書、CI）は不要である。その場合は PR の
「画面の変更」に **UI 変更なし** と書いて、確認済みであることを残す。

Issue handoff SDDでは、親Issueに`ui`ラベルがある場合だけUI workflowを使う。workflowは
`plan -> design -> tasks -> implement`と進み、`design`で作成した`ui-design.md`を実装と
撮影・評価の入力にする。Phase、task、変更path、拡張子からUIかどうかを推測しない。
正規手順は`docs/agent-workflows/design.md`と`docs/agent-workflows/implement.md`にある。

## 何を載せるか

- 変更後の画面。既存画面の変更なら、変更前と並べて載せる
- 最低 360px、768px、1280px の viewport で撮った画面
- 参照画像がある場合は、参照画像と実装結果を並べた比較画像
- 状態によって見え方が変わるもの（空・読み込み中・エラー・大量件数）は、変わった状態の分だけ
- 操作の流れが変わる場合は、画像に加えて操作手順を1〜3行で書く
- visual review の指摘、その修正後の再撮影、視覚上の残課題

私的なファイル名や実データが写り込まないよう、撮影には確認用のデータを使う。

## visual review の進め方

実装者の作業メモをそのまま自己採点に使わない。撮影後に、ユーザー要求、仕様、参照画像、
360px / 768px / 1280px のスクリーンショットを並べて見直し、視覚的階層、情報密度、余白、
タイポグラフィ、画面全体の調和、hover / active / focus / tap target などを確認する。

指摘があれば修正して再撮影する。指摘が無い場合も「visual review: 指摘なし」と PR 本文に
残す。UI 変更の PR 本文には、確認した viewport、比較画像、visual review と修正内容、
interaction / accessibility の確認結果、残課題を書く。

## 撮り方

### Docker やブラウザが使える手元の環境

```bash
make dev     # もしくは make up
```

ブラウザで `http://localhost:8080`（`make dev` の Vite 側は `http://localhost:5173`）を
開き、通常のスクリーンショット機能で撮る。

### コンテナ内・エージェントの実行環境（画面のない環境）

Chromium と Playwright が使える場合は、起動したサーバーを直接撮影できる。

```bash
make build
MDM_MEDIA_DIR="$PWD/media" MDM_DATA_DIR="$PWD/.local/data" ./bin/mdm &
```

**`MDM_MEDIA_DIR` と `MDM_DATA_DIR` は絶対パスで渡す。** 相対パスを渡すと
「絶対パスではありません」と言って起動しない。

#### ブラウザの用意は環境で分かれる

まず何が入っているかを見る。

```bash
ls "${PLAYWRIGHT_BROWSERS_PATH:-$HOME/.cache/ms-playwright}"
```

**(a) 何も無い（素の環境）**: `npx playwright install chromium` で入れる。以降は
`npx playwright screenshot` も `chromium.launch()`（引数なし）もそのまま動く。

```bash
npx playwright screenshot --viewport-size=1280,800 --wait-for-timeout=1500 \
  http://localhost:8080/ /tmp/ui.png
```

**(b) ブラウザが事前に置いてある（Claude Code on the web のコンテナなど）**: この場合
`npx playwright install` は**実行しない**。ただし `npx --yes playwright` は最新版を取りに
行くので、置いてあるブラウザの版と食い違い、`npx playwright screenshot` も
`chromium.launch()`（引数なし）も
`Executable doesn't exist at .../chromium_headless_shell-<別の数字>/...` で落ちる。
**置いてある実体を `executablePath` で名指しする**。

```js
import { chromium } from "playwright";
const browser = await chromium.launch({
  executablePath: `${process.env.PLAYWRIGHT_BROWSERS_PATH}/chromium`,
});
```

`$PLAYWRIGHT_BROWSERS_PATH/chromium` は版のついた実体への symlink なので、これを使えば
`chromium-<数字>/chrome-linux/chrome` を決め打ちせずに済む（数字は更新で変わる）。
symlink が無ければ `ls` で見えた版つきの名前を直に指す。

script を書くのは、`npx playwright screenshot` で足りないとき（画面幅を変えて何枚も撮る、
操作してから撮る、要素だけを切り出す）でもある。

`make build` は版管理している `web/dist/index.html` を上書きするため
（[TD-002](../exec-plans/tech-debt.md)）、撮影後に `git checkout -- web/dist/index.html`
で戻す。

## どこに置き、どう貼るか

1. 画像を `docs/screenshots/<YYYYMMDD>-<短い名前>.png` としてコミットする
   （PNG、横幅 1280 を目安、1 枚 500KB 以下。履歴に残るため、枚数は要点に絞る）
2. PR 本文からはコミット済みの **raw URL** で参照する。相対パスは PR 本文では画像として
   描画されない。枝は消えるので、URL にはコミット SHA を使う

   ```markdown
   ![一覧画面](https://raw.githubusercontent.com/syudead/vv/<commit-sha>/docs/screenshots/20260912-video-list.png)
   ```

人が手で PR を書く場合は、GitHub の本文欄へ画像をドラッグ&ドロップしてもよい
（リポジトリを太らせずに済む）。エージェントはこの方法を使えないため、上記のコミット
経由で貼る。

## 前後を並べる書き方

| 変更前 | 変更後 |
| --- | --- |
| ![before](https://raw.githubusercontent.com/syudead/vv/<sha>/docs/screenshots/<before>.png) | ![after](https://raw.githubusercontent.com/syudead/vv/<sha>/docs/screenshots/<after>.png) |
