# Design documents

Design documents explain consequential technical decisions and their context.
Add each new document to this index.

## 設計文書の方針

- 文書には正しいことだけを書く。今の実装と食い違う記述は残さず、実装を変えた変更の中で
  直す。
- 何を出すか、何を置いてよいかを制限するのは、設計文書の役目ではない。「〜だけとする」
  「〜は出さない」「〜を増やさないこと」のような縛りは書かず、今どうなっているかと、
  なぜそうしたかを書く。

## Documents

下の `ui-design.md` は日本語の文言を引用している。英語化のあとの画面の文言は、英語のカタログ
（`web/src/i18n/en.ts`）が正本である（[i18n](i18n.md#ui-designmd-の文言)）。

- [Core beliefs](core-beliefs.md)
- [Plan品質の規則: 空欄を埋めるための記述を防ぐ](plan-quality.md)
- [技術選定: MDM（Media Data Management）](tech-stack-selection.md)
- [MOVライブ変換のtrack分離入力](mov-live-transcoding.md)
- [ライブ変換のシークと解析情報の再利用](live-transcode-seek.md)
- [ライブ変換のハードウェアエンコード](hardware-encoding.md)
- [再生の画質](playback-quality.md)
- [シーク用スプライトの生成](seek-sprite-generation.md)
- [動画の隣に置いた字幕ファイル](sidecar-subtitles.md)
- [ライブラリ UI: 見た目の規則と一覧の構成](library-ui.md)
- [画面の文言と書式（i18n）](i18n.md)
- [VVMDM ブランドと画面の UI 設計](../../specs/022-vvmdm-brand/ui-design.md)
- [動画シーク時のサムネイルプレビュー UI](../../specs/009-seek-thumbnail-preview/ui-design.md)
- [一覧画面の hover 動画プレビュー UI](../../specs/010-hover-video-preview/ui-design.md)
- [フォルダ階層をたどる画面の UI](../../specs/011-folder-browser/ui-design.md)
- [動画詳細画面の UI](../../specs/012-video-detail-ia/ui-design.md)
- [動画取り込みの進捗表示 UI](../../specs/012-scan-progress/ui-design.md)（置き場所と操作だけ。示す内容は 024）
- [取り込みの進捗と結果の UI](../../specs/024-import-progress/ui-design.md)
- [一覧とフォルダ画面の検索 UI](../../specs/013-library-search/ui-design.md)
- [動画のタグとタグでの絞り込み UI](../../specs/014-video-tags/ui-design.md)
- [単一アカウント認証とゲストの閲覧 UI](../../specs/016-single-account-auth/ui-design.md)
- [フォルダのグループと続けて再生の UI](../../specs/017-folder-groups/ui-design.md)
- [画質メニューと途切れの警告の UI](../../specs/027-playback-quality/ui-design.md)
- [動画の表示名の編集と今の場面のサムネイル指定の UI](../../specs/029-video-overrides/ui-design.md)
- [同じ動画の別バージョンを束ねる UI](../../specs/030-video-versions/ui-design.md)
- [仮のタグの目印と、確定・却下・却下した名前の UI](../../specs/031-tentative-tags/ui-design.md)
- [動画ページの更新日時・作成日時と「作成日」の並び順の UI](../../specs/033-video-dates/ui-design.md)
