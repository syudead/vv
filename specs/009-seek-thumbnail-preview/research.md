# Research: 動画シーク時のサムネイルプレビュー

既存の技術境界は [ARCHITECTURE.md](../../ARCHITECTURE.md)、FFmpeg と配信の選定は
[tech-stack-selection.md](../../docs/design-docs/tech-stack-selection.md)、現在の代表サムネイル判断は
[002 research](../002-core-video-library/research.md)、論理時間軸は
[008 research](../008-live-mp4-playback/research.md)を継承する。本書は本機能が追加する判断だけを記録する。

## R-301: 静止画は要求時に1枚だけ抽出する

**Decision**: 元動画の論理時刻を受け取り、1 request につき1枚の JPEG を抽出して返す。画像を
server側へ永続化せず、request の中断または短い期限で抽出processも終了する。

**Rationale**: 利用者が実際に示した位置だけを処理するため、長尺動画や未視聴動画へ走査時間と保存量を
先払いしない。入力側の時刻指定で動画全体を先頭から復号せずに済み、仕様の1秒以内の表示に合わせられる。

**Alternatives considered**:

- 取り込み時に全尺のスプライトを生成: `002` で Phase 2 候補として保留されていたが、全動画の尺に
  比例する生成・保存を直列workerへ追加し、プレビューを使わない動画にも費用が発生するため採用しない。
  この決定に合わせ、技術選定文書の Phase 2 記述も要求時フレーム抽出へ更新する
- 要求済みフレームをserverへ保存: 利用位置に比例するcacheの上限・回収・不完全生成の管理が新たに
  必要となるため採用しない

## R-302: 1秒 bucket、短い debounce、中断、版付きcacheを組み合わせる

**Decision**: client は対象時刻を最寄りの1秒へ丸め、同じ秒を同じ版付き URL にする。対象位置を
150ms示し続けた場合だけ取得し、別位置へ移ったら前要求を中断する。応答を表示する直前に最新 bucket と
一致することを確認し、同じ URL の再利用は browser の immutable cache に任せる。

**Rationale**: 1秒 bucket は仕様の時刻誤差を満たしながら同一付近の操作を集約する。debounce と中断は
100回の連続移動を100 processへ直結させず、最新照合は中断と応答完了が競合しても古い画像を表示しない。
content-derived version により、同じ内容の再表示は再利用でき、内容変更後の画像は別 URL になる。

**Alternatives considered**:

- pointer move ごとに取得: 操作頻度がprocess数になり、古い応答の競合も増えるため採用しない
- 5秒以上の粗い bucket: 最大誤差が仕様の1秒を超えるため採用しない
- module内に画像bytesを無期限保持: page lifecycleを越える回収責務とmemory上限が必要になるため採用しない

## R-303: 一覧用サムネイルと別の静止画経路にする

**Decision**: 任意時刻の静止画を専用経路で返し、動画詳細に content-derived version を含む基底 URL を
追加する。既存の一覧サムネイル経路とファイルは変更しない。

**Rationale**: 一覧用は取り込みjobが作った1枚の永続画像、シーク用は時刻入力を持つ一時応答であり、
成功条件、失敗、cache key、process lifecycleが異なる。分離すると一覧の既存契約を変えずに済む。

**Alternatives considered**:

- 既存 `/thumbnail` に時刻queryを追加: 同じ経路で「生成済みfile」と「request時process」が分岐し、
  404・409・cacheの意味がquery有無で変わるため採用しない
- ライブ変換responseからframeを得る: 直接配信では使えず、途中開始sourceのoffsetにも依存するため
  再生方式共通の元動画時間軸にならない
