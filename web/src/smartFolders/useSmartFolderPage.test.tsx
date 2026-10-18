import { act, renderHook, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";

import type { CollectionCriteria } from "../api/collectionCriteria";
import * as api from "../api/smartFolders";
import * as events from "../api/serverEvents";
import { folderCard, video } from "../testing/smartFolderPage";
import { useSmartFolderPage } from "./useSmartFolderPage";

afterEach(() => vi.restoreAllMocks());

it("ページ送りと再読込は条件を維持し、フィルター・ソート変更は先頭へ戻して古い続きを破棄する", async () => {
  vi.spyOn(events, "subscribeServerEvents").mockReturnValue(() => undefined);
  const first: api.SmartFolderItem = { kind: "video", video: video(10) };
  const second: api.SmartFolderItem = { kind: "video", video: video(11) };
  const criteria: CollectionCriteria = { watch: "unwatched", sort: "random", seed: 42 };
  const read = vi
    .spyOn(api, "readSmartFolderPage")
    .mockImplementation(async (_id, _signal, _limit, query, criteria) => ({
      smartFolder: folderCard(1, "Folder"),
      trail: [],
      items: [first],
      nextCursor: criteria?.watch === "watched" ? "new-next" : `${query}-next`,
    }));
  const more = vi
    .spyOn(api, "listSmartFolderItems")
    .mockResolvedValue({ items: [second], nextCursor: "last" });
  const { result, rerender } = renderHook(
    ({ query, criteria }) => useSmartFolderPage(1, query, criteria),
    {
      initialProps: {
        query: "old",
        criteria,
      },
    },
  );
  await waitFor(() => expect(result.current.state.kind).toBe("ready"));
  act(() => result.current.loadMore());
  await waitFor(() =>
    expect(result.current.state).toMatchObject({ data: { items: [first, second] } }),
  );
  expect(more).toHaveBeenLastCalledWith(
    1,
    "old-next",
    expect.any(AbortSignal),
    undefined,
    "old",
    criteria,
  );
  act(() => result.current.refresh());
  await waitFor(() => expect(read).toHaveBeenCalledTimes(2));
  await waitFor(() =>
    expect(result.current.state).toMatchObject({
      kind: "ready",
      data: { items: [first, second] },
    }),
  );
  expect(more).toHaveBeenCalledTimes(2);
  expect(more).toHaveBeenLastCalledWith(
    1,
    "old-next",
    expect.any(AbortSignal),
    undefined,
    "old",
    criteria,
  );
  let finish: (page: api.SmartFolderItemPage) => void = () => undefined;
  more.mockImplementationOnce(
    () =>
      new Promise((resolve) => {
        finish = resolve;
      }),
  );
  act(() => result.current.loadMore());
  const signal = more.mock.calls.at(-1)?.[2];
  rerender({ query: "old", criteria: { watch: "watched", sort: "titleDesc" } });
  await waitFor(() =>
    expect(result.current.state).toMatchObject({
      kind: "ready",
      data: { items: [first], nextCursor: "new-next" },
    }),
  );
  expect(signal?.aborted).toBe(true);
  expect(read).toHaveBeenLastCalledWith(1, expect.any(AbortSignal), undefined, "old", {
    watch: "watched",
    sort: "titleDesc",
  });
  await act(async () => finish({ items: [second] }));
  expect(result.current.state).toMatchObject({
    data: { items: [first], nextCursor: "new-next" },
  });
  expect(read).toHaveBeenLastCalledWith(1, expect.any(AbortSignal), undefined, "old", {
    watch: "watched",
    sort: "titleDesc",
  });
  expect(more).toHaveBeenCalledTimes(3);
});
