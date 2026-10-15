// Package wasmplugin は作品情報プラグイン（WebAssembly モジュール）のアダプタである。
// モジュールを実行せずに検査し、wazero で vv のプロセスの中に切り離して動かす
// （docs/design-docs/work-info-plugins.md、specs/049-work-info-plugins/research.md R-1〜R-5）。
//
// プラグインに渡すのは WASI の標準入出力、時計、乱数と、vv の HTTP のホスト関数だけで、
// ファイル、環境変数、引数は渡さない。時間、メモリ、出力と同時に動く数に上限がある。
//
// 依存してよいのは internal/domain だけで、他の internal/* は参照しない。
// 依存の向きは ARCHITECTURE.md を参照。
package wasmplugin
