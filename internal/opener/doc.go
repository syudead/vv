// Package opener はサーバーの PC で、ファイルを OS の既定のアプリで開く。
//
// OS の既定アプリを起動する os/exec は、ffmpeg／ffprobe の入口である
// internal/media とは別の責務なので、このパッケージに閉じ込める。起動できる
// 環境かどうかは起動時に1度だけ決め、要求のたびには探さない。
//
// 依存してよいのは internal/domain だけで、他の internal/* は参照しない。
// 依存の向きは ARCHITECTURE.md を参照。
package opener
