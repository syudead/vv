import { act, renderHook, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";

import * as groups from "../api/groups";
import * as events from "../api/serverEvents";
import { group, video } from "../testing/groupPage";
import { useGroupPage } from "./useGroupPage";

const first: groups.GroupItem = { id: 101, kind: "video", video: video(10) };
const second: groups.GroupItem = { id: 102, kind: "video", video: video(11) };

afterEach(() => vi.restoreAllMocks());

it("再読込は同じ検索条件で表示件数を補充し、条件変更時は先頭ページから始める", async () => {
  vi.spyOn(events, "subscribeServerEvents").mockReturnValue(() => undefined);
  const detail = vi
    .spyOn(groups, "readGroupPage")
    .mockImplementation(async (_id, _trail, _signal, query) => ({
      group: group(1, "Group"),
      trail: [],
      items: [first],
      nextCursor: `${query}-next`,
    }));
  const more = vi.spyOn(groups, "listGroupItems").mockResolvedValue({ items: [second] });
  const { result, rerender } = renderHook(({ query }) => useGroupPage(1, [9], query), {
    initialProps: { query: "old" },
  });
  await waitFor(() => expect(result.current.state.kind).toBe("ready"));
  act(() => result.current.loadMore());
  await waitFor(() => expect(result.current.confirmed.current).toEqual([first, second]));
  more.mockClear();

  act(() => result.current.refresh());
  await waitFor(() => expect(more).toHaveBeenCalledOnce());
  await waitFor(() =>
    expect(result.current.state).toMatchObject({
      kind: "ready",
      data: { items: [first, second], nextCursor: undefined },
    }),
  );
  expect(detail).toHaveBeenLastCalledWith(1, [9], expect.any(AbortSignal), "old");
  expect(more).toHaveBeenLastCalledWith(
    1,
    "old-next",
    expect.any(AbortSignal),
    undefined,
    "old",
  );
  more.mockClear();

  rerender({ query: "new" });
  await waitFor(() =>
    expect(result.current.state).toMatchObject({
      kind: "ready",
      data: { items: [first], nextCursor: "new-next" },
    }),
  );
  expect(detail).toHaveBeenLastCalledWith(1, [9], expect.any(AbortSignal), "new");
  expect(more).not.toHaveBeenCalled();
});
