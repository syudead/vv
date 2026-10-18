import { renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { beaconProgress, saveProgress } from "../api/client";
import { type PlaybackProgressStatus, useProgressSaving } from "./useProgressSaving";

vi.mock("../api/client", () => ({
  saveProgress: vi.fn(() => Promise.resolve({})),
  beaconProgress: vi.fn(),
}));

const save = vi.mocked(saveProgress);
const beacon = vi.mocked(beaconProgress);

interface HookProps {
  id: number;
  owner: boolean;
  status: PlaybackProgressStatus;
}

const notPositioned: PlaybackProgressStatus = { positioned: false, ended: false };
const positioned: PlaybackProgressStatus = { positioned: true, ended: false };
const ended: PlaybackProgressStatus = { positioned: true, ended: true };

function renderSaving(initial: Partial<HookProps> = {}) {
  return renderHook(
    (props: HookProps) => useProgressSaving(props.id, props.owner, props.status),
    {
      initialProps: { id: 7, owner: true, status: notPositioned, ...initial },
    },
  );
}

function leave() {
  window.dispatchEvent(new Event("pagehide"));
}

function savedIds(): (string | undefined)[] {
  return save.mock.calls.map((call) => call[2]);
}

describe("useProgressSaving の視聴の識別子", () => {
  beforeEach(() => {
    save.mockClear();
    beacon.mockClear();
  });

  it("最初の再生より前の保存と離脱時の送信は識別子を持たない", () => {
    const { result } = renderSaving({ status: positioned });

    result.current.savePlayerProgress(5_000, true);
    leave();

    expect(save).toHaveBeenCalledWith(7, 5_000, undefined);
    expect(beacon).toHaveBeenCalledWith(7, 5_000, undefined);
  });

  it("positioned の前の最初の再生では、報告まで送らず、報告後に落ち着いた位置で 1 回すぐ送る", () => {
    const { result, rerender } = renderSaving();

    result.current.rememberProgress(0);
    result.current.markPlayed();
    expect(save).not.toHaveBeenCalled();

    leave();
    expect(beacon).toHaveBeenCalledWith(7, 0, undefined);

    result.current.rememberProgress(42_000);
    rerender({ id: 7, owner: true, status: positioned });

    expect(save).toHaveBeenCalledOnce();
    const [videoId, positionMs, playbackId] = save.mock.calls[0] ?? [];
    expect(videoId).toBe(7);
    expect(positionMs).toBe(42_000);
    expect(playbackId).toHaveLength(36);
  });

  it("positioned のあとの最初の再生では、その場で識別子を付けた保存を送る", () => {
    const { result } = renderSaving({ status: positioned });

    result.current.rememberProgress(12_000);
    result.current.markPlayed();

    expect(save).toHaveBeenCalledOnce();
    expect(save.mock.calls[0]?.[1]).toBe(12_000);
    expect(save.mock.calls[0]?.[2]).toHaveLength(36);
  });

  it("positioned のあとでも位置をまだ知らなければ、最初の位置を知ったときに識別子を付けて送る", () => {
    const { result } = renderSaving({ status: positioned });

    result.current.markPlayed();
    expect(save).not.toHaveBeenCalled();

    result.current.rememberProgress(0);
    result.current.savePlayerProgress(5_000, false);

    const playbackId = save.mock.calls[0]?.[2];
    expect(playbackId).toHaveLength(36);
    expect(save.mock.calls.map((call) => call.slice(0, 3))).toEqual([
      [7, 0, playbackId],
      [7, 5_000, playbackId],
    ]);
  });

  it("位置を知る前の最初の保存は、識別子を付けた最初の保存としてすぐ送る", () => {
    const { result } = renderSaving({ status: positioned });

    result.current.markPlayed();
    result.current.savePlayerProgress(400, false);

    expect(save).toHaveBeenCalledOnce();
    expect(save.mock.calls[0]?.[1]).toBe(400);
    expect(save.mock.calls[0]?.[2]).toHaveLength(36);
  });

  it("以後の保存、一時停止時の保存、離脱時の送信は同じ識別子を持つ", () => {
    const { result } = renderSaving({ status: positioned });
    result.current.rememberProgress(1_000);
    result.current.markPlayed();
    const playbackId = save.mock.calls[0]?.[2];

    result.current.savePlayerProgress(6_000, false);
    result.current.savePlayerProgress(8_000, true);
    leave();
    result.current.markPlayed();
    result.current.savePlayerProgress(15_000, false);

    expect(savedIds()).toEqual([playbackId, playbackId, playbackId, playbackId]);
    expect(beacon).toHaveBeenCalledWith(7, 8_000, playbackId);
  });

  it("同じ動画でプレイヤーを作り直しても識別子は変わらない", () => {
    const { result, rerender } = renderSaving({ status: positioned });
    result.current.rememberProgress(1_000);
    result.current.markPlayed();
    const playbackId = save.mock.calls[0]?.[2];

    rerender({ id: 7, owner: true, status: notPositioned });
    result.current.rememberProgress(20_000);
    result.current.markPlayed();
    rerender({ id: 7, owner: true, status: positioned });
    result.current.savePlayerProgress(26_000, false);

    expect(savedIds()).toEqual([playbackId, playbackId]);
  });

  it("ended の保存は識別子を持ち、次の play（Replay）では新しい識別子になる", () => {
    const { result, rerender } = renderSaving({ status: positioned });
    result.current.rememberProgress(1_000);
    result.current.markPlayed();
    const first = save.mock.calls[0]?.[2];

    // The player saves ended synchronously before the subsequent render.
    result.current.savePlayerProgress(90_000, true);
    rerender({ id: 7, owner: true, status: ended });
    expect(savedIds()).toEqual([first, first]);

    leave();
    expect(beacon).toHaveBeenLastCalledWith(7, 90_000, undefined);

    rerender({ id: 7, owner: true, status: positioned });
    result.current.rememberProgress(0);
    result.current.markPlayed();

    const second = save.mock.calls[2]?.[2];
    expect(second).toHaveLength(36);
    expect(second).not.toBe(first);
  });

  it("別の動画では新しい識別子になり、前の動画の離脱時の送信は前の識別子を持つ", () => {
    const { result, rerender } = renderSaving({ status: positioned });
    result.current.rememberProgress(1_000);
    result.current.markPlayed();
    const first = save.mock.calls[0]?.[2];

    rerender({ id: 8, owner: true, status: notPositioned });
    expect(beacon).toHaveBeenLastCalledWith(7, 1_000, first);

    result.current.rememberProgress(2_000);
    result.current.markPlayed();
    rerender({ id: 8, owner: true, status: positioned });
    expect(save).toHaveBeenLastCalledWith(8, 2_000, expect.any(String));
    const second = save.mock.calls.at(-1)?.[2];
    expect(second).not.toBe(first);

    rerender({ id: 7, owner: true, status: notPositioned });
    result.current.rememberProgress(3_000);
    result.current.markPlayed();
    rerender({ id: 7, owner: true, status: positioned });
    const third = save.mock.calls.at(-1)?.[2];
    expect(save).toHaveBeenLastCalledWith(7, 3_000, expect.any(String));
    expect(third).not.toBe(first);
  });

  it("ゲストでは識別子を作らず、保存も送らない", () => {
    const { result } = renderSaving({ owner: false, status: positioned });

    result.current.rememberProgress(1_000);
    result.current.markPlayed();
    result.current.savePlayerProgress(6_000, true);
    leave();

    expect(save).not.toHaveBeenCalled();
    expect(beacon).not.toHaveBeenCalled();
  });
});
