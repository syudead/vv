import { sendRequest } from "../api/client";

/**
 * PlaybackFailureKind は、再試行を使い切ったあとに見る人へ伝える失敗の種類である。
 *
 * - `network`: サーバーに届かない（回線が切れた・遅すぎて途切れた）。
 * - `decode`: データは届いたが、映像として読めなかった（ファイルが壊れている）。
 * - `source`: サーバーには届くが、動画を出せなかった（ファイルが動いた・消えた、
 *   変換を始められなかった、ブラウザが形式に対応していない）。
 */
export type PlaybackFailureKind = "network" | "decode" | "source";

/**
 * MediaErrorClass は `video` 要素の誤りの分け方である。`ambiguous` は誤りの番号だけでは
 * 決まらず、サーバーに届くかを確かめて `network` か `source` に分ける。
 */
export type MediaErrorClass = "network" | "decode" | "ambiguous";

/**
 * classifyMediaError は MediaError の番号を分ける。
 *
 * 2（MEDIA_ERR_NETWORK）は読み込みの途中で通信が切れたこと、3（MEDIA_ERR_DECODE）は
 * 届いたデータが読めなかったことを表す。1（MEDIA_ERR_ABORTED）は読み込みを打ち切られた
 * ことで、ページが自分で打ち切ったのではないので通信の失敗と同じく扱う。
 * 4（MEDIA_ERR_SRC_NOT_SUPPORTED）は形式に対応していないときだけでなく、読み込みの最初の
 * 要求が届かなかったとき（Chrome）や、サーバーが誤りを返したときにも出るので決めない。
 * video.js 自身の番号（負の値）や番号の無い誤りも同じく決めない。
 */
export function classifyMediaError(code: number | undefined): MediaErrorClass {
  switch (code) {
    case 1:
    case 2:
      return "network";
    case 3:
      return "decode";
    default:
      return "ambiguous";
  }
}

/**
 * reconnectDelaysMs は通信が切れたときに同じ位置から読み込み直すまでの待ちである。
 * 間を広げながら合わせて約 30 秒試し、それでもつながらなければ失敗を伝える。
 */
export const reconnectDelaysMs: readonly number[] = [1000, 2000, 4000, 8000, 15000];

/** reconnectDelay は count 回目（0 から）の読み込み直しまでの待ちで、使い切ったら null。 */
export function reconnectDelay(count: number): number | null {
  return reconnectDelaysMs[count] ?? null;
}

/**
 * recoveredAfterMs は、読み込み直したあとにこれだけ再生が進んだら、回線が戻ったとみなして
 * 読み込み直しの回数を数え直す長さである。すぐにまた切れる回線で、待ちが短いまま
 * 読み込み直しを繰り返さないためである。
 */
export const recoveredAfterMs = 10_000;

/** reachabilityTimeoutMs はサーバーに届くかを確かめる要求の期限である。 */
const reachabilityTimeoutMs = 5000;

/**
 * serverReachable はサーバーに届くか（`/api/health` が何かを返すか）を確かめる。
 * 端末が回線につながっていないと分かっているときは要求を出さない。期限までに返らない
 * ときも届かないとみなす。signal で打ち切ったときは false で解決する。
 */
export async function serverReachable(signal?: AbortSignal): Promise<boolean> {
  if (typeof navigator !== "undefined" && navigator.onLine === false) return false;
  const controller = new AbortController();
  const abort = () => controller.abort();
  signal?.addEventListener("abort", abort);
  const timer = setTimeout(abort, reachabilityTimeoutMs);
  try {
    await sendRequest("/api/health", { cache: "no-store", signal: controller.signal });
    return true;
  } catch {
    return false;
  } finally {
    clearTimeout(timer);
    signal?.removeEventListener("abort", abort);
  }
}
