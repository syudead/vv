# 実行計画: 設定画面

- ステータス: 実装中（#69）
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

検証では、同じcontentを2つの登録rootへ置いた状態で片方を変更・削除しても、もう片方の
locationから動画、job、thumbnail、再生位置を継続利用できることを確認する。
削除locationのjob失敗を残存videoへ書き込まないことと、folder操作後の遅延cleanupが再生成済み
thumbnailを削除しないことも確認する。

## 検証

- 各Issueに記載したunit・contract・UI test
- `make generate`後の生成差分検査
- `make check`
- 360px、768px、1280pxのscreenshotとvisual review
- symlink、部分I/O失敗、stale job write-back、再生履歴維持の回帰確認

### 実施済み

- #67: `go test ./...`
- #67: 旧schemaからのmigrationでvideo ID、location、job、playback progressを維持
- #67: 重複contentの別location維持、stale job write-back拒否、0 folder scan拒否
- #67: reporting failureとroot I/O failureでmissing locationを削除しないことを確認
- #67: PR #73をfeature branchへマージし、全CIとレビュー指摘対応を完了
- #68: PR #74をfeature branchへマージし、全CIとレビュー指摘対応を完了
- #69: `/settings`、行単位の追加・変更・削除、server directory picker、0件・失敗・走査中・競合状態を実装
- #69: Vitest 72件、TypeScript、Prettier、Go test/vet、local-dev testsを完走
- #69: 360/768/1280pxで一覧・empty・picker・削除確認を撮影し、横scroll、重なり、focus trap、Escape、focus復帰を確認

## 進捗

- [x] Spec
- [x] Plan
- [x] Design
- [x] #67
- [x] #68
- [ ] #69
- [ ] 統合検証と`main`向けPR

## 決定

技術判断は [research.md](../../../specs/007-settings-screen/research.md)、データ差分は
[data-model.md](../../../specs/007-settings-screen/data-model.md)、API差分は
[settings-api.md](../../../specs/007-settings-screen/contracts/settings-api.md)を参照する。
