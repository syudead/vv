---
source: docs/design-docs/index.md
sourceHash: 2e4b3c4a754d5128cb765464fb43f762f94f96b6427c13377512a85d54c08bc7
---

# 設計文書 {#design-documents}

設計文書は、影響の大きい技術的な決定とその背景を説明する。新しい文書はそれぞれこの索引に加える。

## 設計文書の方針 {#design-document-policy}

- 文書には正しいことだけを書く。現在の実装と食い違う記述は残しておかず、実装を変えた変更の中で直す。
- 何を表示または配置してよいかを制限するのは、設計文書の役目ではない。「X だけ」「Y を表示しない」「Z をこれ以上足さない」のような制約は書かず、今どうなっているかとその理由を書く。

## 文書 {#documents}

以下の `ui-design.md` 文書は日本語の画面の文言を引用している。英語化の後は、英語のカタログ (`web/src/i18n/en.ts`) が画面の文言の正である ([i18n](i18n.md#wording-in-ui-designmd))。

- [基本の信条](core-beliefs.md)
- [文書の品質: 型に沿った技術英語](writing-quality.md)
- [文書の日本語訳](japanese-translation.md)
- [Plan の品質ルール: 枠を埋めるためだけの文を書かない](plan-quality.md)
- [技術選定: MDM (Media Data Management)](tech-stack-selection.md)
- [MOV のライブ変換でのトラック別の入力](mov-live-transcoding.md)
- [ライブ変換のシークとプローブの再利用](live-transcode-seek.md)
- [ライブ変換のハードウェアエンコード](hardware-encoding.md)
- [Windows デスクトップアプリ (VVMDM.exe)](windows-app.md)
- [プロセスのライフサイクル: 起動と停止の順序](process-lifecycle.md)
- [フォルダの監視: 変更のあったメディアフォルダの取り込み](folder-watching.md)
- [再生画質](playback-quality.md)
- [シーク用スプライトの生成](seek-sprite-generation.md)
- [隣の字幕ファイル](sidecar-subtitles.md)
- [vv デザインシステム](design-system.md)
- [ライブラリ UI: 視覚ルールと一覧のレイアウト](library-ui.md)
- [画面の文言と書式 (i18n)](i18n.md)
- [VVMDM のブランドと画面の UI 設計](../../specs/022-vvmdm-brand/ui-design.md)
- [シーク中のサムネイルプレビュー: UI](../../specs/009-seek-thumbnail-preview/ui-design.md)
- [一覧でのホバー時の動画プレビュー: UI](../../specs/010-hover-video-preview/ui-design.md)
- [フォルダを閲覧する画面: UI](../../specs/011-folder-browser/ui-design.md)
- [動画ページ: UI](../../specs/012-video-detail-ia/ui-design.md)
- [動画の取り込みの進捗: UI](../../specs/012-scan-progress/ui-design.md) (配置と操作だけ。何を表示するかは 024 にある)
- [取り込みの進捗と結果: UI](../../specs/024-import-progress/ui-design.md)
- [ライブラリ画面とフォルダ画面での検索: UI](../../specs/013-library-search/ui-design.md)
- [動画のタグとタグによる絞り込み: UI](../../specs/014-video-tags/ui-design.md)
- [単一アカウントの認証とゲストの閲覧: UI](../../specs/016-single-account-auth/ui-design.md)
- [フォルダのグループと連続再生: UI](../../specs/017-folder-groups/ui-design.md)
- [画質メニューと停滞の警告: UI](../../specs/027-playback-quality/ui-design.md)
- [動画の表示名の編集と、現在のフレームのサムネイルとしての使用: UI](../../specs/029-video-overrides/ui-design.md)
- [同じ動画のバージョンをまとめる: UI](../../specs/030-video-versions/ui-design.md)
- [仮のタグのマーク、および確定、却下、却下した名前: UI](../../specs/031-tentative-tags/ui-design.md)
- [カードのサムネイルの下端に沿ったスクラブ: UI](../../specs/032-card-scrub-preview/ui-design.md)
- [動画ページの更新日と作成日、および「Date created」の並び順: UI](../../specs/033-video-dates/ui-design.md)
- [動画とグループのお気に入りのマーク、切り替え、絞り込み、並び順: UI](../../specs/035-favorites/ui-design.md)
- [規模の大きいタグ管理画面 (上部バー、帯、タブ、並び順、選択、一括操作): UI](../../specs/036-tag-admin-scale/ui-design.md)
- [shadcn/ui 上の vv デザインシステム (基礎、コンポーネント、ページのパターン): UI](../../specs/038-design-system/ui-design.md)
- [カードの `+N` の裏に隠れたタグを、何個でも見えて押せるようにする: UI](../../specs/041-tag-overflow-list/ui-design.md)
- [変更されたメディアフォルダの自動取り込み (設定、問題の知らせ、静かな結果の知らせ): UI](../../specs/042-folder-watch-import/ui-design.md)
