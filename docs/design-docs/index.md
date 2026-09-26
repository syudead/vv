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

- [Core beliefs](core-beliefs.md)
- [Plan品質の規則: 空欄を埋めるための記述を防ぐ](plan-quality.md)
- [技術選定: MDM（Media Data Management）](tech-stack-selection.md)
- [MOVライブ変換のtrack分離入力](mov-live-transcoding.md)
- [ライブ変換のシークと解析情報の再利用](live-transcode-seek.md)
- [Issue handoff SDD: agentを固定しない一工程ずつの実行](sdd-loop-harness.md)
- [ライブラリ UI: 見た目の規則と一覧の構成](library-ui.md)
- [一覧画面の hover 動画プレビュー UI](../../specs/010-hover-video-preview/ui-design.md)
- [動画詳細画面の UI](../../specs/012-video-detail-ia/ui-design.md)
- [動画取り込みの進捗表示 UI](../../specs/012-scan-progress/ui-design.md)
- [一覧とフォルダ画面の検索 UI](../../specs/013-library-search/ui-design.md)
- [動画のタグとタグでの絞り込み UI](../../specs/014-video-tags/ui-design.md)
- [単一アカウント認証とゲストの閲覧 UI](../../specs/016-single-account-auth/ui-design.md)
- [フォルダのグループと続けて再生の UI](../../specs/017-folder-groups/ui-design.md)
