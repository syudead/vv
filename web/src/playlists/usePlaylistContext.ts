import { useCallback, useEffect, useRef, useState } from "react";

import { isAborted, RequestFailed } from "../api/client";
import {
  getPlaylist,
  getPlaylistPlayback,
  type Playlist,
  type PlaylistPlayback,
} from "../api/playlists";
import { t } from "../i18n";
import { useToast } from "../ui/Toast";
import {
  choosePair,
  maxPairReads,
  pairAgrees,
  type PlaylistPair,
  type PlaylistQuery,
} from "./playlistPlayback";

export type PlaylistPanelState =
  { kind: "loading" } | { kind: "ready"; playlist: Playlist } | { kind: "failed" };

/**
 * Missing context or guests stay off; either request's 404 drops context and stops after the
 * current video. Initial failures expose Retry, while ready drives playback navigation.
 */
export type PlaylistContextStatus = "off" | "reading" | "ready" | "failed" | "dropped";

export interface PlaylistContext {
  status: PlaylistContextStatus;
  playback: PlaylistPlayback | undefined;
  panel: PlaylistPanelState;
  /** A mismatch with the current readKey means refresh is still pending. */
  settledKey: number;
  retry: () => void;
}

interface Data {
  key: string;
  playlistId: number;
  status: PlaylistContextStatus;
  playback: PlaylistPlayback | undefined;
  panel: PlaylistPanelState;
  settledKey: number;
}

const loadingPanel: PlaylistPanelState = { kind: "loading" };

function isNotFound(reason: unknown): boolean {
  return reason instanceof RequestFailed && reason.status === 404;
}

/**
 * Owners read playback and panel together, retrying version mismatches up to the limit. A 404
 * drops context until reload; retain prior responses during refresh and panel rows within the same
 * playlist.
 */
export function usePlaylistContext({
  query,
  videoId,
  owner,
  readKey,
}: {
  query: PlaylistQuery | null;
  videoId: number;
  owner: boolean;
  readKey: number;
}): PlaylistContext {
  const toast = useToast();
  const toastRef = useRef(toast);
  toastRef.current = toast;
  const active = query !== null && owner && videoId > 0;
  const playlistId = query?.playlistId ?? 0;
  const itemId = query?.itemId ?? 0;
  const key = active ? `${String(playlistId)}:${String(itemId)}:${String(videoId)}` : "";
  const [data, setData] = useState<Data | null>(null);
  const [retryToken, setRetryToken] = useState(0);
  const latest = useRef<Data | null>(null);
  latest.current = data;

  useEffect(() => {
    if (!active) return;
    const current = latest.current;
    // Do not reread dropped context until a page reload.
    if (current?.key === key && current.status === "dropped") return;
    const controller = new AbortController();
    const signal = controller.signal;

    const commit = (update: (previous: Data | null) => Data) => {
      if (signal.aborted) return;
      setData((previous) => update(previous?.key === key ? previous : null));
    };

    const run = async () => {
      const pairs: PlaylistPair[] = [];
      for (let attempt = 0; attempt < maxPairReads; attempt += 1) {
        const [playback, playlist] = await Promise.allSettled([
          getPlaylistPlayback(playlistId, itemId, videoId, signal),
          getPlaylist(playlistId, signal),
        ]);
        if (signal.aborted) return;
        const reasons = [playback, playlist].flatMap((result) =>
          result.status === "rejected" ? [result.reason as unknown] : [],
        );
        if (reasons.some(isAborted)) return;
        if (reasons.some(isNotFound)) {
          commit(() => ({
            key,
            playlistId,
            status: "dropped",
            playback: undefined,
            panel: loadingPanel,
            settledKey: readKey,
          }));
          toastRef.current(t.player.playlist.gone);
          return;
        }
        if (playback.status === "rejected") {
          // Retain previous responses on transient failure; otherwise show Retry again, including failed retries.
          commit((previous) =>
            previous?.playback !== undefined
              ? {
                  ...previous,
                  panel:
                    previous.panel.kind === "loading"
                      ? { kind: "failed" }
                      : previous.panel,
                  settledKey: readKey,
                }
              : {
                  key,
                  playlistId,
                  status: "failed",
                  playback: undefined,
                  panel: { kind: "failed" },
                  settledKey: readKey,
                },
          );
          return;
        }
        if (playlist.status === "rejected") {
          // Never combine new playback with an old panel order; retain an agreeing prior pair or show panel Retry.
          commit((previous) =>
            previous?.playback !== undefined && previous.panel.kind === "ready"
              ? { ...previous, settledKey: readKey }
              : {
                  key,
                  playlistId,
                  status: "ready",
                  playback: playback.value,
                  panel: { kind: "failed" },
                  settledKey: readKey,
                },
          );
          return;
        }
        const pair = { playback: playback.value, playlist: playlist.value };
        pairs.push(pair);
        if (pairAgrees(pair)) break;
      }
      const chosen = choosePair(pairs);
      if (chosen === null) return;
      commit(() => ({
        key,
        playlistId,
        status: "ready",
        playback: chosen.playback,
        panel: { kind: "ready", playlist: chosen.playlist },
        settledKey: readKey,
      }));
    };
    void run();
    return () => controller.abort();
    // retryToken triggers reads only for the panel's Retry action.
  }, [active, itemId, key, playlistId, readKey, retryToken, videoId]);

  const retry = useCallback(() => {
    setData((previous) =>
      previous === null
        ? null
        : {
            ...previous,
            status: previous.status === "failed" ? "reading" : previous.status,
            panel: loadingPanel,
          },
    );
    setRetryToken((value) => value + 1);
  }, []);

  if (!active) {
    return {
      status: "off",
      playback: undefined,
      panel: loadingPanel,
      settledKey: readKey,
      retry,
    };
  }
  if (data?.key === key) {
    return {
      status: data.status,
      playback: data.playback,
      panel: data.panel,
      settledKey: data.settledKey,
      retry,
    };
  }
  // Retain panel rows while moving between items in the same playlist.
  return {
    status: "reading",
    playback: undefined,
    panel:
      data?.playlistId === playlistId && data.panel.kind === "ready"
        ? data.panel
        : loadingPanel,
    settledKey: -1,
    retry,
  };
}
