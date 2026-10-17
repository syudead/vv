---
source: docs/design-docs/index.md
sourceHash: af51994f80108cae160fecb7cc5fc85ae0749ecf40901f5e93a7d339f8d5ea7b
---

# 設計文書 {#design-documents}

設計文書は、影響の大きい技術的な決定とその背景を説明する。新しい文書はそれぞれこの索引に加える。完了した SDD 成果物は参照文書ではない。削除前に、残すべき決定をここに記録する。

## 設計文書の方針 {#design-document-policy}

- 文書には正しいことだけを書く。現在の実装と食い違う記述は残しておかず、実装を変えた変更の中で直す。
- 何を表示または配置してよいかを制限するのは、設計文書の役目ではない。「X だけ」「Y を表示しない」「Z をこれ以上足さない」のような制約は書かず、今どうなっているかとその理由を書く。

## 文書 {#documents}

- [基本の信条](core-beliefs.md)
- [文書の品質: 型に沿った技術英語](writing-quality.md)
- [文書の日本語訳](japanese-translation.md)
- [Plan の品質ルール: 枠を埋めるためだけの文を書かない](plan-quality.md)
- [技術選定: MDM (Media Data Management)](tech-stack-selection.md)
- [MOV のライブ変換でのトラック別の入力](mov-live-transcoding.md)
- [ライブ変換のシークとプローブの再利用](live-transcode-seek.md)
- [取り込み時のプローブから得るメディア情報](media-information.md)
- [ライブ変換のハードウェアエンコード](hardware-encoding.md)
- [Windows デスクトップアプリ (VVMDM.exe)](windows-app.md)
- [プロセスのライフサイクル: 起動と停止の順序](process-lifecycle.md)
- [フォルダの監視: 変更のあったメディアフォルダの取り込み](folder-watching.md)
- [再生画質](playback-quality.md)
- [シーク用スプライトの生成](seek-sprite-generation.md)
- [隣の字幕ファイル](sidecar-subtitles.md)
- [判定モデルによる自動タグ付け](auto-tagging.md)
- [AI 層: 分類器のポートとプロバイダ](ai-layer.md)
- [重複の候補を自動で判定する](duplicate-judging.md)
- [アクティビティログ](activity.md)
- [サムネイル編集のセッション](thumbnail-editing.md)
- [作品情報プラグイン: サンドボックス、制限、ネットワーク](work-info-plugins.md)
- [vv デザインシステム](design-system.md)
- [ライブラリ UI: 視覚ルールと一覧のレイアウト](library-ui.md)
- [関連動画と前後の動画](related-videos.md)
- [画面の文言と書式 (i18n)](i18n.md)
- [テスト方針: 共通の規則](testing.md)
- [テストでの時間と同期](testing-time.md)
- [Web のテストレベル](web-testing.md)
- [テストと CI の品質監視](test-quality-monitoring.md)
- [複雑度と重複](complexity-and-duplication.md)
- [認証と公開範囲](authentication.md)
- [カタログの整理と永続的な識別子](catalog-organization.md)
- [取り込みの進捗と回復](import-pipeline.md)
