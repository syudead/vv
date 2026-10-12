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

/** PlaylistPanelState はパネルに出すプレイリストの応答の状態である。 */
export type PlaylistPanelState =
  { kind: "loading" } | { kind: "ready"; playlist: Playlist } | { kind: "failed" };

/**
 * PlaylistContextStatus は動画ページのプレイリストの文脈の段階である
 * （specs/044-playlists/ui-design.md「Video page in a playlist」）。
 *
 * - `off`: クエリが無いか、ゲスト。ページは今までどおり
 * - `reading`: 開いてから最初の応答まで。行と矢印は描かない
 * - `ready`: 再生の応答がある。行・矢印・予告・終わりはこれに従う
 * - `failed`: 最初の再生の読みが 404 以外で失敗した。パネルが Retry を出す
 * - `dropped`: どちらかの読みが 404。文脈を落とし、今の動画の後で止まる
 */
export type PlaylistContextStatus = "off" | "reading" | "ready" | "failed" | "dropped";

export interface PlaylistContext {
  status: PlaylistContextStatus;
  playback: PlaylistPlayback | undefined;
  panel: PlaylistPanelState;
  /** 最後に読み終えた `readKey`。ページが渡した値と違う間は、読み直しの途中である。 */
  settledKey: number;
  /** パネルの Retry。2 つの応答を組として読み直す。 */
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
 * usePlaylistContext は、URL の `?playlist=&item=` と再生の応答から、動画ページの
 * プレイリストの文脈を作る（plan の Structure decision、research.md R-6・R-7）。
 *
 * - 所有者だけが読む。ゲストとクエリの無いときは何も送らない（R-8）。
 * - 開いたとき・`readKey` が変わったとき（動画の終わり、予告が次の動画の消えたのを知ったとき）
 *   に `GET /api/playlists/{id}/playback` と `GET /api/playlists/{id}` を同時に読み、
 *   `updatedAt` のそろった組だけを使う。そろわなければ上限まで読み直す（One order）。
 * - どちらかが 404 なら文脈を落とし、トーストを 1 度出す。以後はその項目について読まない。
 * - 読み直しの間は前の応答を保つ。別の項目へ移っても、同じプレイリストならパネルの行を保つ。
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
    // 落とした文脈は読み直さない（再読み込みで読み直す）。
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
          // 一時的な失敗。前の応答があれば保ち、無ければパネルが Retry を出す。Retry の読みが
          // 失敗したときは、パネルを読み込み中のまま残さず、もう一度 Retry を出す。
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
          // 再生とパネルは 1 つの組で示す（One order）。前の組がそろっていればそれを保ち、
          // 新しい再生の応答だけを古いパネルの並びと合わせない。前の組が無ければ、新しい
          // 再生の応答に従い、パネルは Retry を出す。
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
    // retryToken はパネルの Retry で読み直すためだけに入れる。
  }, [active, itemId, key, playlistId, readKey, retryToken, videoId]);

  const retry = useCallback(() => {
    // 読み直す間はパネルを読み込み中の形にする。再生の応答の無い失敗は、最初の読みと同じ段階へ戻す。
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
  // 別の項目へ移った直後。同じプレイリストなら、応答が来るまでパネルの行を保つ。
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
