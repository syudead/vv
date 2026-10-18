import { act, renderHook, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";

import type { CollectionCriteria } from "../api/collectionCriteria";
import * as groups from "../api/groups";
import * as events from "../api/serverEvents";
import { group, video } from "../testing/groupPage";
import { useGroupPage } from "./useGroupPage";

const first: groups.GroupItem = { id: 101, kind: "video", video: video(10) };
const second: groups.GroupItem = { id: 102, kind: "video", video: video(11) };

afterEach(() => vi.restoreAllMocks());

it("条件付きの次ページ・再読込を維持し、条件変更前の応答を破棄する", async () => {
  vi.spyOn(events, "subscribeServerEvents").mockReturnValue(() => undefined);
  const criteria: CollectionCriteria = { watch: "unwatched", sort: "random", seed: 42 };
  const detail = vi
    .spyOn(groups, "readGroupPage")
    .mockImplementation(async (_id, _trail, _signal, _query, conditions) => ({
      group: group(1, "Group"),
      trail: [],
      items: [first],
      nextCursor: `${conditions?.watch}-next`,
    }));
  const more = vi
    .spyOn(groups, "listGroupItems")
    .mockResolvedValue({ items: [second], nextCursor: "last" });
  const { result, rerender } = renderHook(
    ({ conditions }) => useGroupPage(1, [9], "cat", conditions),
    {
      initialProps: { conditions: criteria },
    },
  );
  await waitFor(() => expect(result.current.state.kind).toBe("ready"));
  act(() => result.current.loadMore());
  await waitFor(() => expect(result.current.confirmed.current).toEqual([first, second]));
  more.mockClear();

  act(() => result.current.refresh());
  await waitFor(() => expect(more).toHaveBeenCalledOnce());
  await waitFor(() =>
    expect(result.current.state).toMatchObject({
      kind: "ready",
      data: { items: [first, second], nextCursor: "last" },
    }),
  );
  expect(detail).toHaveBeenLastCalledWith(
    1,
    [9],
    expect.any(AbortSignal),
    "cat",
    criteria,
  );
  expect(more).toHaveBeenLastCalledWith(
    1,
    "unwatched-next",
    expect.any(AbortSignal),
    undefined,
    "cat",
    criteria,
  );
  more.mockClear();

  let finish: (page: groups.GroupItemPage) => void = () => undefined;
  more.mockImplementationOnce(
    () =>
      new Promise((resolve) => {
        finish = resolve;
      }),
  );
  act(() => result.current.loadMore());
  const signal = more.mock.calls.at(-1)?.[2];
  const nextCriteria: CollectionCriteria = { watch: "watched", sort: "titleDesc" };
  rerender({ conditions: nextCriteria });
  await waitFor(() =>
    expect(result.current.state).toMatchObject({
      kind: "ready",
      data: { items: [first], nextCursor: "watched-next" },
    }),
  );
  expect(detail).toHaveBeenLastCalledWith(
    1,
    [9],
    expect.any(AbortSignal),
    "cat",
    nextCriteria,
  );
  expect(signal?.aborted).toBe(true);
  await act(async () => finish({ items: [second] }));
  expect(result.current.state).toMatchObject({
    data: { items: [first], nextCursor: "watched-next" },
  });
  expect(more).toHaveBeenCalledOnce();
});

it("再読込中に移動を送ったら古い順序を採用せず、移動完了後に再取得する", async () => {
  vi.spyOn(events, "subscribeServerEvents").mockReturnValue(() => undefined);
  const initial = { group: group(1, "Group"), trail: [], items: [first] };
  let finishRead: (page: Awaited<ReturnType<typeof groups.readGroupPage>>) => void = () =>
    undefined;
  const read = vi
    .spyOn(groups, "readGroupPage")
    .mockResolvedValueOnce(initial)
    .mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          finishRead = resolve;
        }),
    )
    .mockResolvedValueOnce({ ...initial, items: [second, first] });
  const { result } = renderHook(() => useGroupPage(1, []));
  await waitFor(() => expect(result.current.state.kind).toBe("ready"));
  act(() => result.current.refresh());
  await waitFor(() => expect(read).toHaveBeenCalledTimes(2));

  let finishMove: () => void = () => undefined;
  act(() => {
    result.current.movesSent.current += 1;
    result.current.moves.current = new Promise((resolve) => {
      finishMove = resolve;
    });
  });
  await act(async () => finishRead({ ...initial, trail: [], items: [second] }));
  expect(read).toHaveBeenCalledTimes(2);
  expect(result.current.confirmed.current).toEqual([first]);
  expect(result.current.state).toMatchObject({ data: { items: [first] } });

  await act(async () => finishMove());
  await waitFor(() => expect(read).toHaveBeenCalledTimes(3));
  await waitFor(() => expect(result.current.confirmed.current).toEqual([second, first]));
  expect(result.current.state).toMatchObject({ data: { items: [second, first] } });
});
