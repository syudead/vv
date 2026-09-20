# Research: 設定画面

既存の技術選定は [ARCHITECTURE.md](../../ARCHITECTURE.md) と
[技術選定文書](../../docs/design-docs/tech-stack-selection.md)を引き継ぐ。以下は本機能で追加する
判断だけを記録する。

## R-701: 設定の正本

**Decision**: SQLite にメディアフォルダの singleton 行を保存し、行がない初回だけ
`MDM_MEDIA_DIR` から初期化する。

**Rationale**: サーバー処理とすべてのブラウザーが同じ永続値を使うため。

**Alternatives considered**: JSON ファイル、localStorage、毎起動の環境変数優先は、正本が
分かれるため採用しない。

## R-702: 走査と配信への反映

**Decision**: 設定更新と走査開始を同じ排他境界で直列化し、走査は開始時の値を固定する。
動画配信は要求ごとに現行値を参照する。保存済みフォルダが消失してもサーバーは起動し、
設定画面から修正可能にする。

**Rationale**: 走査途中の切替と、変更後に旧ルートを配信することを防ぐため。

**Alternatives considered**: 起動時の固定値、走査途中の動的参照、保存時の自動走査は要求を
満たさない。

## R-703: 同時更新

**Decision**: 設定に整数 `version` を持たせ、PUT の条件付き更新で古い画面からの保存を
`409` にする。

**Rationale**: 複数タブから新しい値を黙って上書きしないため。

**Alternatives considered**: last-write-wins は仕様の競合要件を満たさない。

## R-704: 設定画面

**Decision**: 既存 AppShell 内の `/settings` に、メディアフォルダ入力と保存操作だけを置く。

**Rationale**: 1項目でも情報設計上は設定画面であり、既存のナビゲーションと操作規則を
そのまま利用できるため。

**Alternatives considered**: モーダルやメディア専用画面は、設定画面という要求と直接URLを失う。
