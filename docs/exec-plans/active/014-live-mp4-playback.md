# 実行計画: MP4 ライブ変換による動画再生

- ステータス: 実装中
- 最終更新: 2026-09-22
- Parent Issue: #100
- Feature branch: `codex/live-mp4-playback-feature`

## 目的

直接配信を第一候補として維持し、再生できない動画を保存なしのMP4ライブ変換へ有限回切り替える。
要求と設計の正本は[Spec](../../../specs/008-live-mp4-playback/spec.md)と
[Plan](../../../specs/008-live-mp4-playback/plan.md)とし、本書は実装順序と進捗だけを記録する。

## 実装単位

- #103 fragmented MP4 ライブ変換と配信 API
- #104 直接配信からライブ変換へ有限切り替えするプレイヤー
- #105 動画形式 matrix の E2E

各IssueのPRは`codex/live-mp4-playback-feature`をbaseとし、統合PRだけを`main`へ向ける。

## 検証

- 各Issueに記載したunit、contract、browser test
- `task check`
- `task test-e2e`
- 実装PRのreviewとCI
- 統合前に最新`main`をreview済みsub-branch経由で取り込む

## 進捗

- [x] Spec
- [x] Plan
- [x] 子Issue作成
- [x] #103
- [x] #104
- [x] #105
- [ ] 統合検証と`main`向けPR（検証完了、PR作成待ち）

## 決定の記録

Phase 2以降で承認済みartifactを補足する実装判断が必要になった場合だけ追記する。
