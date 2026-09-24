// Package app はユースケース（アプリケーション層）を保持する。
//
// 走査の開始と実行、取り込みの各ジョブの処理、動画1件の応答に要る判断と
// 関連動画の組み立てがここにある。保存・外部コマンド・生成物の操作は、この
// パッケージが宣言する interface 越しに使い、どの実装が渡されるかは cmd/mdm が
// 決める。そのため net/http・database/sql・os/exec・SQLite ドライバと、
// internal/ 配下のアダプタ（httpapi・store・media・artifacts・opener・scanner・jobs）と
// 変化の配り先（eventbus）の import は禁止されている。この禁止は
// .golangci.yml の depguard で機械的に強制される。
//
// 依存の向き:
//
//	cmd → internal/{app,httpapi,store,media,artifacts,opener,scanner,jobs,eventbus} → internal/domain
//
// 詳細は ARCHITECTURE.md の "Intended dependency direction" を参照。
package app
