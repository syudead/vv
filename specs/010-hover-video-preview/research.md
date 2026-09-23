# Research: 一覧画面の hover 動画プレビュー

## Inherited Sources

- Web 境界、API 正本、検査入口は [plan.md](plan.md#technical-context) の canonical definitions を継承する。
- 要求と受け入れ条件は [GitHub Issue #134](https://github.com/syudead/vv/issues/134) を正本とする。

## Decisions

### Hover できる pointer だけで preview を開始する

**Decision**: `pointerenter` ごとに `event.pointerType === "mouse"` を確認して preview を mouse/trackpad の hover に限定し、keyboard focus、touch contact、row view では開始しない。

**Rationale**: 親 Issue は hover できない入力環境とキーボード操作で自動再生しないこと、行ビューを対象外にすることを明示している。イベント自身の `pointerType` なら touch contact を除外しつつ、タッチ主体端末へ接続した mouse を許可できる。既存カードは focus、selection、click navigation をすでに持つため、preview はそれらより低い優先順位の補助表示に留める。

**Alternatives considered**: `(hover: hover) and (pointer: fine)` を eligibility に使う案は主入力しか表さず、タッチ主体端末へ接続した mouse を除外するため採用しない。focus でも preview を開始する案は、キーボード利用者の移動だけで動画を動かし、focus ring と Enter 遷移の確認を難しくするため採用しない。touch long press preview は親 Issue の対象外なので採用しない。

### 直接 stream だけを preview source にする

**Decision**: preview の video source は既存 `streamUrl(video.id)` を使い、`video.playable === true` のカードだけを対象にする。

**Rationale**: 親 Issue はブラウザで直接再生できない動画を hover のためにライブ変換しないと定めている。既存 API contract は `Video.playable` と `/api/videos/{id}/stream` をすでに公開しており、OpenAPI や generated types を増やさずに preview できる。

**Alternatives considered**: `transcodeUrl(video.id)` へ fallback する案は、一覧 hover が本再生より重いライブ変換を起動し、対象外のブラウザ非対応動画 preview を実質的に追加するため採用しない。新しい preview endpoint を作る案は、既存 stream で足りる直接再生ケースに API と生成物の変更を増やすため採用しない。

### Preview lifecycle はグリッドカードが所有する

**Decision**: preview の待機 timer、video element、play rejection handling、停止処理は `VideoCard` または同じ library boundary の小さな helper が所有する。

**Rationale**: preview はカードのサムネイル領域内で完結する表示であり、一覧全体の paging、filtering、selection、scroll restoration の state と独立している。React の unmount cleanup を使えば、一覧再取得、絞り込み、並び替え、画面遷移でも resource を閉じられる。

**Alternatives considered**: `LibraryPage` に active preview id を持たせる案は、一覧のデータ取得・選択管理に media playback lifecycle を混ぜ、カード単体の unit test がしづらくなるため採用しない。
