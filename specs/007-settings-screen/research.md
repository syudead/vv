# Research: 設定画面

共通の技術選定は [ARCHITECTURE.md](../../ARCHITECTURE.md) と
[技術選定文書](../../docs/design-docs/tech-stack-selection.md)を引き継ぐ。

## R-701: SQLiteだけを設定の正本にする

**Decision**: メディアフォルダは初回未設定でSQLiteに保持する。`MDM_MEDIA_DIR` は移行元や
初期値としても使わず廃止する。

**Rationale**: UI設定と環境変数という2つの正本を残さず、設定方法を画面へ一本化するため。

**Alternatives considered**: 環境変数の恒久併用、一度だけのbootstrap、`/media` の暗黙設定は、
画面で選んでいない値を有効にするため採用しない。

## R-702: サーバー側directory browserを提供する

**Decision**: ブラウザー標準pickerではなく、APIが返すサーバーfilesystemのdirectoryを辿る
folder pickerを実装する。path省略時はLinuxのrootまたはWindowsのdriveを返し、以後は親と
直下directoryを返す。ファイル内容とファイル名は返さない。

**Rationale**: ブラウザー標準pickerはclient端末を対象とする。利用者にサーバー絶対パスを
手入力させず、実在する選択肢から指定できるようにするため。

**Alternatives considered**: text inputは誤入力を招くため不採用。ブラウザー標準pickerは
対象filesystemが異なる。server上のfolder作成は別機能なので含めない。

## R-703: 選択時と保存時の両方で検証する

**Decision**: directory listing時に読取可能性を確認し、PUT時にも存在・directory・readableを
再検証する。APIが返すpathはOSの絶対・正規化済み表現とする。

**Rationale**: 一覧表示後にdirectoryが消える競合を扱い、clientから任意文字列を送られても
無効な保存値を作らないため。

## R-704: 走査と配信への反映

**Decision**: 未設定時は走査を開始しない。設定更新と走査開始を直列化し、走査は開始時の値を
固定する。動画配信は要求ごとに現行値を参照する。

**Rationale**: 未設定や走査途中の切替を許さず、現在設定のroot外を配信しないため。

## R-705: versionで同時更新を検出する

**Decision**: 設定に整数versionを持たせ、PUTの条件付き更新で古い画面からの保存を409にする。

**Rationale**: 複数画面から新しい値を黙って上書きしないため。
