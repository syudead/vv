# Research: 動画シーク時のサムネイルプレビュー

> シーク用サムネイルの保存形式・間隔・配信 API は、
> [021-seek-thumbnail-sprite](../021-seek-thumbnail-sprite/plan.md) でスプライトシートに置き換えた。
> 現在の形は [021 の契約](../021-seek-thumbnail-sprite/contracts/seek-sprite-api.md) を正本とし、この文書の間隔・画像の単位・要求の形はそれを参照する。

## R-301: シーク画像は取り込みjobで事前生成する

**Decision**: 既存のthumbnail background jobで、一覧用画像に加えてシーク用の画像（間隔・形式は[021 の契約](../021-seek-thumbnail-sprite/contracts/seek-sprite-api.md)）を
生成し、`MDM_DATA_DIR/thumbnails/seek`へcontent key単位で保存する。既存動画はmigrationで同じjobへ
一度だけ再投入する。HTTP request中にFFmpegを起動しない。

**Rationale**: requestごとのFFmpeg起動とseek/decodeは、画像が表示されるまで数秒かかり、連続した
シーク操作に適さなかった。取り込み時に一度だけ動画を順次decodeすれば、再生中は生成済みfileのreadだけで
応答できる。生成途中は一時directoryに隔離し、完了後にrenameするため、不完全なcacheを配信しない。

**Alternatives considered**:

- request時抽出: 初回表示が遅く、操作頻度にprocess起動が連動するため廃止
- lazy disk cache: 2回目以降しか速くならず、初回シークの体験を改善しないため不採用
- 1秒間隔: 保存量と初回生成時間が増えるため採らない（現在の間隔は[021 の契約](../021-seek-thumbnail-sprite/contracts/seek-sprite-api.md)）

## R-302: clientとserverは同じ区間の規則を使う

**Decision**: clientとserverは対象時刻から画像を選ぶ同じ規則（[021 の契約](../021-seek-thumbnail-sprite/contracts/seek-sprite-api.md)）を使う。画像が変わるときに
即時要求し、中断、最新位置の照合、decode後の差し替え、版付きimmutable cacheを使う。

**Rationale**: 同じ区間のpointer移動を同じURLへまとめ、不要なreadと描画競合を避ける。時刻表示自体は
実際のpointer位置へ即時追従するため、シーク精度は粗くならない。次画像のdecode完了までは表示済み画像を
維持し、通信・decode待ちの黒い面を連続操作へ挟まない。

## R-303: 一覧画像と保存場所を分離する

**Decision**: 一覧用の代表画像は従来のpathに保ち、シーク画像は`seek/<prefix>/<content-key>/`
へ保存する（中身は[021 の契約](../021-seek-thumbnail-sprite/contracts/seek-sprite-api.md)）。どちらも再構築可能なindex dataとして扱う。

**Rationale**: 一覧画像は1動画1枚、シーク画像は1動画に複数fileで、配信とcleanupの単位が異なる。
同じbackground jobで生成しても保存境界を分けることで既存一覧APIを変更しない。

## R-304: 既存動画は直列workerで段階的に生成する

**Decision**: migrationは既存動画をthumbnail queueへ一度だけ再投入し、既存の直列workerで順番に生成する。
1動画の上限は30分とし、失敗しても一覧用thumbnailと再生を維持する。cache容量と初回完了時間は動画尺に
比例するため、生成中は409へ縮退し、ライブラリ全体の完了を起動条件にしない。

**Rationale**: 並列decodeはCPU・disk負荷を急増させる。画像の量は動画尺に応じて増える（上限は[021 の契約](../021-seek-thumbnail-sprite/contracts/seek-sprite-api.md)）ため、運用時は`thumbnails/seek`の容量を監視できる再構築可能dataとして扱う。
