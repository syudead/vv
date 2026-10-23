import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { Video } from "../../api/client";
import { ToastProvider } from "../../ui/Toast";
import MergeDialog from "./MergeDialog";

const virtualVideo = {
  id: 7,
  virtual: true,
  title: "仮の動画",
  contentVersion: "virtual:3",
} as Video;

function fileVideo(id: number, title: string): Video {
  return {
    id,
    title,
    contentVersion: `key${String(id)}`,
    durationMs: 65_000,
    folder: { rootId: 1, path: "shows/2026" },
  } as Video;
}

const first = fileVideo(21, "第一話");
const second = fileVideo(22, "第二話");

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function failure(status: number, code: string, reason: string): Response {
  return json({ code, reason, message: reason }, status);
}

const server = {
  list: vi.fn<(url: URL) => Response>(),
  merge: vi.fn<(body: unknown) => Response>(),
};

function renderDialog() {
  const props = { onClose: vi.fn(), onMerged: vi.fn(), onReload: vi.fn() };
  render(
    <ToastProvider>
      <MergeDialog video={virtualVideo} {...props} />
    </ToastProvider>,
  );
  return props;
}

async function chooseAndMerge(title: string) {
  const user = userEvent.setup();
  const dialog = await screen.findByRole("dialog", { name: "Merge into a file video" });
  await user.click(
    await within(dialog).findByRole("option", { name: new RegExp(title) }),
  );
  await user.click(within(dialog).getByRole("button", { name: "Merge" }));
  return { user, dialog };
}

describe("MergeDialog", () => {
  beforeEach(() => {
    server.list.mockReset();
    server.list.mockImplementation(() => json({ items: [first, second], total: 2 }));
    server.merge.mockReset();
    vi.stubGlobal(
      "fetch",
      vi.fn<typeof fetch>((input, init) => {
        const url = new URL(String(input), "http://localhost");
        if (url.pathname === "/api/folders")
          return Promise.resolve(
            json({ folders: [{ rootId: 1, name: "Anime", rootPath: "/media/Anime" }] }),
          );
        if (url.pathname === "/api/videos") return Promise.resolve(server.list(url));
        if (url.pathname === "/api/virtual-videos/7/merge" && init?.method === "POST")
          return Promise.resolve(server.merge(JSON.parse(String(init.body))));
        return Promise.resolve(json({}));
      }),
    );
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("ファイルのある動画だけを検索し、選んだ 1 行に印を付けて、読んだ版で送る", async () => {
    server.merge.mockReturnValue(json(second));
    const props = renderDialog();
    const user = userEvent.setup();
    const dialog = await screen.findByRole("dialog", { name: "Merge into a file video" });
    const merge = within(dialog).getByRole("button", { name: "Merge" });
    expect(merge).toHaveProperty("disabled", true);
    expect(
      await within(dialog).findByRole("option", {
        name: /第一話.*1:05 · Anime\/shows\/2026/,
      }),
    ).toBeDefined();

    await user.type(
      within(dialog).getByRole("combobox", { name: "Search file videos" }),
      "第",
    );
    const reads = server.list.mock.calls.map(([url]) => url.searchParams);
    expect(reads.at(-1)?.get("query")).toBe("第");
    for (const read of reads) {
      expect(read.get("kind")).toBe("file");
      expect(read.get("limit")).toBe("20");
    }

    await user.click(within(dialog).getByRole("option", { name: /第一話/ }));
    await user.click(within(dialog).getByRole("option", { name: /第二話/ }));
    const chosen = within(dialog)
      .getAllByRole("option")
      .filter((row) => row.getAttribute("aria-checked") === "true");
    expect(chosen.map((row) => row.textContent)).toEqual([
      expect.stringContaining("第二話"),
    ]);
    await user.click(merge);

    expect(await screen.findByText('Merged into "第二話"')).toBeDefined();
    expect(server.merge).toHaveBeenCalledWith({
      contentVersion: "virtual:3",
      videoId: 22,
      videoContentVersion: "key22",
    });
    expect(props.onMerged).toHaveBeenCalledWith(expect.objectContaining({ id: 22 }));
  });

  it("一致が無ければ No file video matches、読み取りの失敗は Retry で読み直す", async () => {
    server.list.mockReturnValueOnce(failure(500, "internal", "internal"));
    server.list.mockReturnValueOnce(json({ items: [], total: 0 }));
    renderDialog();
    const user = userEvent.setup();
    const dialog = await screen.findByRole("dialog");
    expect(await within(dialog).findByText("Couldn't load file videos")).toBeDefined();
    await user.click(within(dialog).getByRole("button", { name: "Retry" }));
    expect(await within(dialog).findByText("No file video matches")).toBeDefined();
  });

  it.each([
    [
      failure(409, "conflict", "description_too_long"),
      "The two descriptions together are too long. Shorten one and try again.",
    ],
    [
      failure(409, "conflict", "too_many_urls"),
      "The two videos have more than 20 links together. Remove some and try again.",
    ],
    [
      failure(404, "not_found", "video_not_found"),
      "One of the videos is no longer in the library.",
    ],
    [failure(409, "conflict", "virtual_video"), /^Couldn't merge: /],
  ])("マージの失敗 %#: ダイアログと選択を残して文言を出す", async (response, text) => {
    server.merge.mockReturnValue(response);
    const props = renderDialog();
    const { dialog } = await chooseAndMerge("第一話");
    const alert = await within(dialog).findByRole("alert");
    expect(alert.textContent).toMatch(text);
    expect(within(alert).queryByRole("button", { name: "Reload" })).toBeNull();
    expect(
      within(dialog)
        .getByRole("option", { name: /第一話/ })
        .getAttribute("aria-checked"),
    ).toBe("true");
    expect(props.onMerged).not.toHaveBeenCalled();
  });

  it("content_changed は Reload を出し、押すとページを読み直させる", async () => {
    server.merge.mockReturnValue(failure(409, "conflict", "content_changed"));
    const props = renderDialog();
    const { user, dialog } = await chooseAndMerge("第一話");
    const alert = await within(dialog).findByRole("alert");
    expect(alert.textContent).toContain(
      "One of the videos changed in another tab. Reload and try again.",
    );
    await user.click(within(alert).getByRole("button", { name: "Reload" }));
    expect(props.onReload).toHaveBeenCalledTimes(1);
  });
});
