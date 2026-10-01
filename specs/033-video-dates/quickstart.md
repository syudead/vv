# Quickstart: 作成日時をファイルシステムで確かめる

親 Issue #630 の受け入れ条件のうち、自動テストが実際のファイルシステムの作成日時に依存できないもの
（受け入れ条件 4・5）を、動いているサーバーで確かめる手順である。ほかの受け入れ条件は `task check` が
走らせる store・scanner・httpapi・Vitest の試験で確かめる。結果は、走査の単位
（「走査がファイルの作成日時を読み、所在に記録する」）の実装 PR の本文に残す。

## 前提

- [docs/how-to/development.md](../../docs/how-to/development.md) のとおり `task dev` で起動し、所有者で
  ログインし、メディアフォルダを 1 つ登録している。以下 `BASE=http://localhost:8080`。
- 作成日時を持つファイルシステム（Linux の ext4・xfs・btrfs、macOS の APFS、Windows の NTFS）の上に
  メディアフォルダがあること。

## 手順

1. メディアフォルダに動画を 1 本コピーし、OS の作成日時を控える（Linux: `stat -c %w <path>`、
   macOS: `stat -f %SB <path>`、Windows: エクスプローラーのプロパティ）。
2. スキャンを実行し、動画ページを開く。情報欄の作成日時が 1 の値と一致する（受け入れ条件 4）。
   `curl -H "Authorization: Bearer $TOKEN" "$BASE/api/v1/videos/lookup?path=<path>"` の `fileCreatedAt`
   も同じである（受け入れ条件 7）。
3. 作成日時を持たないファイルシステム（`tmpfs`、または `stat -c %w` が `-` を返す場所）の上に
   メディアフォルダを登録して同じ手順を踏むと、作成日時がそのファイルの更新日時（`stat -c %y`）と
   一致する（受け入れ条件 5）。
4. この feature より前に登録した動画（移行前のデータベース）は、スキャンの前は作成日時が更新日時と
   同じで、スキャンの後に OS の作成日時になる（Edge Case「既に登録済みの動画」）。

## 期待する結果

上のそれぞれが成り立つ。
