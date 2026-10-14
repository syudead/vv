// 動画の作品情報の編集画面の経路（specs/047-video-work-info/ui-design.md「Editor」）。
// 画面の読み込み（app/App.tsx）と所有者だけの画面の判定（auth/AuthGate.tsx）が共有する。

/** videoEditPath は動画の編集画面の経路である。 */
export function videoEditPath(videoId: number): string {
  return `/videos/${String(videoId)}/edit`;
}

/** isVideoEditPath は動画の編集画面（`/videos/:id/edit`）の経路かを返す。 */
export function isVideoEditPath(pathname: string): boolean {
  return /^\/videos\/[^/]+\/edit\/?$/i.test(pathname);
}
