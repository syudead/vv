import { act, renderHook } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { RequestFailed } from "./client";
import type { ItemPage } from "./readThrough";
import { type PagedSource, usePagedDetail } from "./usePagedDetail";

interface Data extends ItemPage<number> {
  name: string;
}

interface Pending<T> {
  signal: AbortSignal;
  resolve: (value: T) => void;
  reject: (error: unknown) => void;
}

function deferredSource() {
  const reads: (Pending<Data> & { atLeast: number })[] = [];
  const pages: (Pending<ItemPage<number>> & { cursor: string })[] = [];
  const onMore = vi.fn();
  const source: PagedSource<Data, number> = {
    read: (_id, atLeast, signal) =>
      new Promise((resolve, reject) => reads.push({ atLeast, signal, resolve, reject })),
    more: (_id, cursor, signal) =>
      new Promise((resolve, reject) => pages.push({ cursor, signal, resolve, reject })),
    append: (current, page) => [...current, ...page],
    onMore,
  };
  return { source, reads, pages, onMore };
}

const gone = new RequestFailed(404, "not_found", "x");
const failed = new RequestFailed(500, "internal", "x");

async function settle(run: () => void) {
  await act(async () => {
    run();
    await Promise.resolve();
  });
}

describe("usePagedDetail", () => {
  it("id が無ければ無くなった状態で、読みに行かない", async () => {
    const { source, reads } = deferredSource();
    const { result } = renderHook(() => usePagedDetail(null, source));
    expect(result.current.state).toEqual({ kind: "gone" });

    await settle(() => result.current.read(false));
    await settle(() => result.current.loadMore());
    expect(reads).toHaveLength(0);
  });

  it("新しい読み込みを始めると、古い読み込みの答えと失敗を捨てる", async () => {
    const { source, reads } = deferredSource();
    const { result } = renderHook(() => usePagedDetail(1, source));
    await settle(() => result.current.read(false));
    await settle(() => result.current.read(true));
    await settle(() => result.current.read(true));
    expect(reads[0]?.signal.aborted).toBe(true);
    expect(reads[1]?.signal.aborted).toBe(true);

    await settle(() => reads[0]?.resolve({ name: "古い", items: [1] }));
    await settle(() => reads[1]?.reject(failed));
    expect(result.current.state).toEqual({ kind: "loading" });

    await settle(() => reads[2]?.resolve({ name: "新しい", items: [1, 2] }));
    expect(result.current.state).toMatchObject({
      kind: "ready",
      data: { name: "新しい" },
    });
  });

  it("静かな読み直しは並んでいる分を求め、失敗しても中身を残す。404 は無くなった状態", async () => {
    const { source, reads } = deferredSource();
    const { result } = renderHook(() => usePagedDetail(1, source));
    await settle(() => result.current.read(false));
    await settle(() => reads[0]?.resolve({ name: "a", items: [1, 2, 3] }));

    await settle(() => result.current.read(true));
    expect(reads[1]?.atLeast).toBe(3);
    await settle(() => reads[1]?.reject(failed));
    expect(result.current.state.kind).toBe("ready");

    await settle(() => result.current.read(false));
    await settle(() => reads[2]?.reject(failed));
    expect(result.current.state).toEqual({ kind: "failed" });

    await settle(() => result.current.read(true));
    await settle(() => reads[3]?.reject(gone));
    expect(result.current.state).toEqual({ kind: "gone" });
  });

  it("beforeRead が見せないと答えた中身は捨てる", async () => {
    const { source, reads } = deferredSource();
    const accept = vi.fn(() => false);
    const { result } = renderHook(() =>
      usePagedDetail(1, { ...source, beforeRead: () => accept }),
    );
    await settle(() => result.current.read(false));
    await settle(() => reads[0]?.resolve({ name: "a", items: [] }));

    expect(accept).toHaveBeenCalledTimes(1);
    expect(result.current.state).toEqual({ kind: "loading" });
  });

  it("続きは読めた後だけ、読み込み中でなく続きがあるときだけ読み、足して onMore を呼ぶ", async () => {
    const { source, reads, pages, onMore } = deferredSource();
    const { result } = renderHook(() => usePagedDetail(1, source));
    await settle(() => result.current.loadMore());
    await settle(() => result.current.read(false));
    await settle(() => result.current.loadMore());
    expect(pages).toHaveLength(0);

    await settle(() => reads[0]?.resolve({ name: "a", items: [1], nextCursor: "c1" }));
    await settle(() => result.current.loadMore());
    await settle(() => result.current.loadMore());
    expect(pages).toHaveLength(1);
    expect(result.current.more).toBe("loading");

    await settle(() => pages[0]?.resolve({ items: [2] }));
    expect(onMore).toHaveBeenCalledWith([2]);
    expect(result.current.state).toMatchObject({ data: { items: [1, 2] } });
    expect(result.current.more).toBe("idle");

    await settle(() => result.current.loadMore());
    expect(pages).toHaveLength(1);
  });

  it("続きの失敗は failed、404 は無くなった状態、読み直しが始まった続きは捨てる", async () => {
    const { source, reads, pages } = deferredSource();
    const { result } = renderHook(() => usePagedDetail(1, source));
    await settle(() => result.current.read(false));
    await settle(() => reads[0]?.resolve({ name: "a", items: [1], nextCursor: "c1" }));

    await settle(() => result.current.loadMore());
    await settle(() => pages[0]?.reject(failed));
    expect(result.current.more).toBe("failed");

    await settle(() => result.current.loadMore());
    await settle(() => result.current.read(true));
    expect(pages[1]?.signal.aborted).toBe(true);
    await settle(() => pages[1]?.reject(failed));
    expect(result.current.state).toMatchObject({ data: { items: [1] } });

    await settle(() => reads[1]?.resolve({ name: "a", items: [1], nextCursor: "c1" }));
    await settle(() => result.current.loadMore());
    await settle(() => pages[2]?.reject(gone));
    expect(result.current.state).toEqual({ kind: "gone" });
    expect(result.current.more).toBe("idle");
  });

  it("無くなった状態にすると、読んでいる続きを捨てる", async () => {
    const { source, reads, pages } = deferredSource();
    const { result } = renderHook(() => usePagedDetail(1, source));
    await settle(() => result.current.read(false));
    await settle(() => reads[0]?.resolve({ name: "a", items: [1], nextCursor: "c1" }));
    await settle(() => result.current.loadMore());

    const pending = pages[0];
    await settle(() => result.current.markGone());
    expect(pending?.signal.aborted).toBe(true);
    await settle(() => pending?.resolve({ items: [9] }));
    expect(result.current.state).toEqual({ kind: "gone" });
  });

  it("update は読めた中身だけを変え、stop は読み込みを捨てる", async () => {
    const { source, reads } = deferredSource();
    const { result } = renderHook(() => usePagedDetail(1, source));
    await settle(() => result.current.update((data) => ({ ...data, name: "読む前" })));
    expect(result.current.state).toEqual({ kind: "loading" });

    await settle(() => result.current.read(false));
    await settle(() => reads[0]?.resolve({ name: "a", items: [] }));
    await settle(() => result.current.update((data) => ({ ...data, name: "b" })));
    expect(result.current.state).toMatchObject({ data: { name: "b" } });

    await settle(() => result.current.read(true));
    await settle(() => result.current.stop());
    expect(reads[1]?.signal.aborted).toBe(true);
  });
});
