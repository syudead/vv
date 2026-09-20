# 実行計画: 設定画面

- ステータス: 計画・設計中
- 最終更新: 2026-09-21
- Parent Issue: #62
- Feature branch: `codex/feature-settings-screen`

## 目的

メディアフォルダだけを扱う設定画面を追加し、複数folderを1件ずつ安全に管理できるようにする。
要求と実装内容の正本は [Spec](../../../specs/007-settings-screen/spec.md) と
[Plan](../../../specs/007-settings-screen/plan.md) とし、この文書には進捗と検証結果だけを記録する。

## 実装単位

- #67 メディアフォルダ個別操作・対象別無効化・安全な走査
- #68 メディアフォルダ個別操作・ディレクトリ選択 API
- #69 設定画面とメディアフォルダ個別操作 UI

各IssueのPRは`codex/feature-settings-screen`をbaseとし、統合PRだけを`main`へ向ける。

## 検証

- 各Issueに記載したunit・contract・UI test
- `make generate`後の生成差分検査
- `make check`
- 360px、768px、1280pxのscreenshotとvisual review
- symlink、部分I/O失敗、stale job write-back、再生履歴維持の回帰確認

## 進捗

- [x] Spec
- [x] Plan
- [ ] Design
- [ ] #67
- [ ] #68
- [ ] #69
- [ ] 統合検証と`main`向けPR

## 決定

技術判断は [research.md](../../../specs/007-settings-screen/research.md)、データ差分は
[data-model.md](../../../specs/007-settings-screen/data-model.md)、API差分は
[settings-api.md](../../../specs/007-settings-screen/contracts/settings-api.md)を参照する。
