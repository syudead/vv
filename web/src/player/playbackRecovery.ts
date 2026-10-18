import { sendRequest } from "../api/client";

export type PlaybackFailureKind = "network" | "decode" | "source";

/** Ambiguous media errors require a reachability probe to distinguish network from source failure. */
export type MediaErrorClass = "network" | "decode" | "ambiguous";

/** Treat aborted loads as network failures. Unsupported-source, custom, and missing codes need probing because browsers also use them for request failures. */
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

/** Back off over roughly 30 seconds before reporting a connection failure. */
export const reconnectDelaysMs: readonly number[] = [1000, 2000, 4000, 8000, 15000];

export function reconnectDelay(count: number): number | null {
  return reconnectDelaysMs[count] ?? null;
}

/** Reset retries only after sustained actual playback, excluding seeks, to prevent rapid loops on unstable connections. */
export const recoveredAfterMs = 10_000;

/** Advance retries if metadata never arrives, even without an explicit network error. */
export const reloadTimeoutMs = 15_000;

const probeTimeoutMs = 5000;

/** Distinguish unreachable servers, successful content, and reachable HTTP errors. */
export type ProbeResult = "unreachable" | "ok" | "error";

/** Probe one byte for direct files; use health for transcodes to avoid starting a conversion. Offline or aborted probes resolve unreachable. */
export async function probeMediaSource(
  streamUrl: string | null,
  signal?: AbortSignal,
): Promise<ProbeResult> {
  if (typeof navigator !== "undefined" && navigator.onLine === false)
    return "unreachable";
  const controller = new AbortController();
  const abort = () => controller.abort();
  signal?.addEventListener("abort", abort);
  const timer = setTimeout(abort, probeTimeoutMs);
  try {
    const response = await sendRequest(
      streamUrl ?? "/api/health",
      streamUrl === null
        ? { cache: "no-store", signal: controller.signal }
        : {
            cache: "no-store",
            headers: { Range: "bytes=0-0" },
            signal: controller.signal,
          },
    );
    void response.body?.cancel().catch(() => undefined);
    return response.ok ? "ok" : "error";
  } catch {
    return "unreachable";
  } finally {
    clearTimeout(timer);
    signal?.removeEventListener("abort", abort);
  }
}
