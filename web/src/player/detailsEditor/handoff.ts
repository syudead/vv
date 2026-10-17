import { useEffect, useState } from "react";

import type { Video } from "../../api/client";

// 動画ページと編集画面（/videos/:id/edit）の間で、手元の `Video` を渡す控え
//
//
// 履歴の状態（location.state）には載せない。再読み込みや後から戻ったときに、古い `Video` を
// 今の値として使ってしまうからである。控えはメモリにだけあり、受け取った側が描画の中で
// 読み（StrictMode は初期化を 2 度呼ぶので、読むだけでは消さない）、効果の中で消す。

let toEditor: Video | null = null;
let toPage: PageHandoff | null = null;

/** PageHandoff は、編集画面から戻ったページが受け取る値である。 */
export interface PageHandoff {
  id: number;
  /** 保存の応答。取り消して戻ったときは無い。 */
  video?: Video;
}

/** handVideoToEditor は、入口を押したときのページの `Video` を編集画面へ渡す。 */
export function handVideoToEditor(video: Video): void {
  toEditor = video;
}

/** peekVideoForEditor は、入口から渡された `Video` を返す。別の動画なら返さない。 */
export function peekVideoForEditor(id: number): Video | undefined {
  return toEditor !== null && toEditor.id === id ? toEditor : undefined;
}

/** clearVideoForEditor は、編集画面が受け取った控えを消す。 */
export function clearVideoForEditor(): void {
  toEditor = null;
}

/**
 * handBackToPage は、編集画面を離れるときに動画ページへ渡す。`video` は保存の応答で、
 * 取り消したときは無い。どちらでもページは入口へフォーカスを戻す。
 */
export function handBackToPage(id: number, video?: Video): void {
  toPage = { id, video };
}

/** peekPageHandoff は、編集画面から戻ったときの控えを返す。別の動画なら返さない。 */
export function peekPageHandoff(id: number): PageHandoff | null {
  return toPage !== null && toPage.id === id ? toPage : null;
}

/** clearPageHandoff は、ページが受け取った控えを消す。 */
export function clearPageHandoff(): void {
  toPage = null;
}

/** ReceivedHandoff は、動画ページが開いたときに受け取った控えの使い道である。 */
export interface ReceivedHandoff {
  /** 保存の応答。取り消して戻ったとき、編集画面から戻ったのでないときは無い。 */
  video?: Video;
  /** focusEntry は、videoId の動画のページが編集画面から戻ったところかを返す。 */
  focusEntry: (videoId: number) => boolean;
}

/**
 * usePageHandoff は、動画ページを開いたときの控えを 1 度だけ読み、効果の中で消す。保存の
 * 応答は最初の描画に使い、入口へフォーカスを戻すかは今の動画で決める。
 */
export function usePageHandoff(id: number): ReceivedHandoff {
  const [handoff] = useState(() => peekPageHandoff(id));
  useEffect(() => clearPageHandoff(), []);
  return {
    video: handoff?.video,
    focusEntry: (videoId) => handoff !== null && handoff.id === videoId,
  };
}
