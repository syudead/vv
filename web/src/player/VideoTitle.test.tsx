import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { Video } from "../api/client";
import { TooltipProvider } from "../ui/Tooltip";
import VideoTitle from "./VideoTitle";

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

const video: Video = {
  id: 7,
  title: "clip_0042",
  fileTitle: "clip_0042",
  public: false,
  sizeBytes: 1,
  addedAt: "2026-09-01T00:00:00Z",
  playable: true,
  probeState: "done",
  thumbnailState: "done",
  previewState: "done",
  durationMs: 1000,
  videoCodec: "h264",
  tags: [],
};

const named: Video = { ...video, title: "Summer trip", displayName: "Summer trip" };

/** Harness は呼び出し側（VideoPage）と同じく、保存の応答で動画を差し替える。 */
function Harness({
  initial,
  owner = true,
  onStale = () => undefined,
  onSaved,
}: {
  initial: Video;
  owner?: boolean;
  onStale?: () => void;
  onSaved?: (video: Video) => void;
}) {
  const [current, setCurrent] = useState(initial);
  harness.set = setCurrent;
  return (
    <VideoTitle
      video={current}
      owner={owner}
      onSaved={(saved) => {
        onSaved?.(saved);
        setCurrent(saved);
      }}
      onStale={onStale}
    />
  );
}

const harness = { set: (_: Video) => undefined as void };

function renderTitle(props: Parameters<typeof Harness>[0]) {
  return render(
    <TooltipProvider>
      <Harness {...props} />
    </TooltipProvider>,
  );
}

function editButton() {
  return screen.getByRole("button", { name: "Edit name" });
}

function nameInput() {
  return screen.getByRole<HTMLInputElement>("textbox", { name: "Display name" });
}

function sentBodies(fetchMock: ReturnType<typeof vi.fn<typeof fetch>>) {
  return fetchMock.mock.calls.map(([url, init]) => ({
    url: String(url),
    method: init?.method,
    body: typeof init?.body === "string" ? (JSON.parse(init.body) as unknown) : undefined,
  }));
}

describe("VideoTitle", () => {
  const fetchMock = vi.fn<typeof fetch>();
  beforeEach(() => vi.stubGlobal("fetch", fetchMock));
  afterEach(() => {
    vi.unstubAllGlobals();
    fetchMock.mockReset();
  });

  it("ゲストには題名だけを出し、編集の入口とファイル名の行を出さない（受け入れ条件 9）", () => {
    renderTitle({ initial: named, owner: false });
    expect(screen.getByRole("heading", { level: 1, name: "Summer trip" })).toBeDefined();
    expect(screen.queryByRole("button", { name: "Edit name" })).toBeNull();
    expect(screen.queryByText("clip_0042")).toBeNull();
  });

  it("所有者には編集ボタンを出し、表示名が無ければファイル名の行を出さない", () => {
    renderTitle({ initial: video });
    expect(screen.getByRole("heading", { level: 1, name: "clip_0042" })).toBeDefined();
    expect(editButton()).toBeDefined();
    expect(screen.queryByText("File name")).toBeNull();
  });

  it("表示名があれば、題名の下に元のファイル名を従の 1 行で出す（受け入れ条件 7）", () => {
    renderTitle({ initial: named });
    const line = screen.getByTitle("File name: clip_0042");
    expect(line.textContent).toBe("File name clip_0042");
    expect(line.className).toContain("text-fg-muted");
    expect(line.querySelector("a, button")).toBeNull();
  });

  it("編集は今の題名を全選択した入力で始まり、プレースホルダーはファイル名で、ファイル名の行を隠す", async () => {
    const user = userEvent.setup();
    renderTitle({ initial: named });
    await user.click(editButton());
    const input = nameInput();
    expect(input.value).toBe("Summer trip");
    expect(input.placeholder).toBe("clip_0042");
    expect(document.activeElement).toBe(input);
    expect([input.selectionStart, input.selectionEnd]).toEqual([0, 11]);
    expect(input.maxLength).toBe(-1);
    expect(screen.queryByRole("heading", { level: 1 })).toBeNull();
    expect(screen.queryByTitle("File name: clip_0042")).toBeNull();
    expect(screen.getByRole("button", { name: "Save" })).toBeDefined();
    expect(screen.getByRole("button", { name: "Cancel" })).toBeDefined();
  });

  it("Enter で 1 回だけ送り、応答の動画で題名とファイル名の行を置き換え、編集ボタンへフォーカスを返す（受け入れ条件 1・7）", async () => {
    const user = userEvent.setup();
    fetchMock.mockResolvedValueOnce(json(named));
    renderTitle({ initial: video });
    await user.click(editButton());
    await user.clear(nameInput());
    await user.type(nameInput(), "Summer trip{Enter}");

    await screen.findByRole("heading", { level: 1, name: "Summer trip" });
    expect(sentBodies(fetchMock)).toEqual([
      {
        url: "/api/videos/7/display-name",
        method: "PUT",
        body: { displayName: "Summer trip" },
      },
    ]);
    expect(screen.getByTitle("File name: clip_0042")).toBeDefined();
    expect(document.activeElement).toBe(editButton());
  });

  it("空（空白だけ）で保存すると解除として送り、ファイル名由来の題名に戻る（受け入れ条件 2）", async () => {
    const user = userEvent.setup();
    fetchMock.mockResolvedValueOnce(json(video));
    renderTitle({ initial: named });
    await user.click(editButton());
    await user.clear(nameInput());
    await user.type(nameInput(), "   ");
    await user.click(screen.getByRole("button", { name: "Save" }));

    await screen.findByRole("heading", { level: 1, name: "clip_0042" });
    expect(sentBodies(fetchMock)[0]?.body).toEqual({ displayName: "   " });
    expect(screen.queryByTitle("File name: clip_0042")).toBeNull();
  });

  it("表示名の無い動画で題名を変えずに保存しても送らない", async () => {
    const user = userEvent.setup();
    renderTitle({ initial: video });
    await user.click(editButton());
    await user.keyboard("{Enter}");
    expect(fetchMock).not.toHaveBeenCalled();
    expect(screen.getByRole("heading", { level: 1, name: "clip_0042" })).toBeDefined();
  });

  it("前後に空白のあるファイル名の題名は、変えずに保存しても送らない", async () => {
    const user = userEvent.setup();
    const spaced: Video = { ...video, title: " clip_0042 ", fileTitle: " clip_0042 " };
    renderTitle({ initial: spaced });
    await user.click(editButton());
    expect(nameInput().value).toBe(" clip_0042 ");
    await user.keyboard("{Enter}");
    expect(fetchMock).not.toHaveBeenCalled();
    expect(screen.getByRole("heading", { level: 1 }).textContent).toBe(" clip_0042 ");
    expect(screen.queryByTitle(/^File name/)).toBeNull();
  });

  it("Esc と Cancel は送らずに題名へ戻し、編集ボタンへフォーカスを返す", async () => {
    const user = userEvent.setup();
    renderTitle({ initial: named });
    await user.click(editButton());
    await user.type(nameInput(), "x{Escape}");
    expect(screen.getByRole("heading", { level: 1, name: "Summer trip" })).toBeDefined();
    expect(document.activeElement).toBe(editButton());

    await user.click(editButton());
    await user.click(screen.getByRole("button", { name: "Cancel" }));
    expect(screen.getByRole("heading", { level: 1, name: "Summer trip" })).toBeDefined();
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("長すぎる名前は理由を入力の下に出して編集を続け、次の保存で消す（Edge Case「長すぎる」）", async () => {
    const user = userEvent.setup();
    const long = "x".repeat(201);
    fetchMock.mockResolvedValueOnce(
      json(
        {
          code: "invalid_request",
          reason: "display_name_too_long",
          message: "too long",
          limit: 200,
        },
        400,
      ),
    );
    renderTitle({ initial: video });
    await user.click(editButton());
    await user.clear(nameInput());
    await user.click(nameInput());
    await user.paste(long);
    await user.keyboard("{Enter}");

    const alert = await screen.findByRole("alert");
    expect(alert.textContent).toBe(
      "Couldn't save the name: The name can't be longer than 200 characters.",
    );
    expect(nameInput().value).toBe(long);
    expect(nameInput().readOnly).toBe(false);
    expect(screen.queryByRole("heading", { level: 1 })).toBeNull();

    let finish: ((response: Response) => void) | undefined;
    fetchMock.mockImplementationOnce(
      () =>
        new Promise<Response>((resolve) => {
          finish = resolve;
        }),
    );
    await user.keyboard("{Enter}");
    expect(screen.queryByRole("alert")).toBeNull();
    await act(async () => {
      finish?.(json({ ...video, title: long, displayName: long }));
      await Promise.resolve();
    });
  });

  it("送信中は入力を readOnly、Save を aria-disabled と回転の印にし、二重に送らない", async () => {
    const user = userEvent.setup();
    let finish: ((response: Response) => void) | undefined;
    fetchMock.mockImplementationOnce(
      () =>
        new Promise<Response>((resolve) => {
          finish = resolve;
        }),
    );
    renderTitle({ initial: video });
    await user.click(editButton());
    await user.type(nameInput(), "Summer trip{Enter}");

    const save = screen.getByRole("button", { name: "Save" });
    expect(nameInput().readOnly).toBe(true);
    expect(save.getAttribute("aria-disabled")).toBe("true");
    expect(save.hasAttribute("disabled")).toBe(false);
    expect(save.querySelector("svg.animate-spin")).not.toBeNull();
    await user.click(save);
    await user.keyboard("{Enter}");
    expect(fetchMock).toHaveBeenCalledTimes(1);

    await act(async () => {
      finish?.(json(named));
      await Promise.resolve();
    });
    await screen.findByRole("heading", { level: 1, name: "Summer trip" });
  });

  it("送信中に Cancel したら待たずに題名へ戻し、届いた応答は動画の差し替えにだけ使う", async () => {
    const user = userEvent.setup();
    const onSaved = vi.fn();
    let finish: ((response: Response) => void) | undefined;
    fetchMock.mockImplementationOnce(
      () =>
        new Promise<Response>((resolve) => {
          finish = resolve;
        }),
    );
    renderTitle({ initial: video, onSaved });
    await user.click(editButton());
    await user.type(nameInput(), "Summer trip{Enter}");
    await user.click(screen.getByRole("button", { name: "Cancel" }));
    expect(screen.getByRole("heading", { level: 1, name: "clip_0042" })).toBeDefined();

    await act(async () => {
      finish?.(json(named));
      await Promise.resolve();
    });
    await waitFor(() => expect(onSaved).toHaveBeenCalledTimes(1));
    expect(screen.getByRole("heading", { level: 1, name: "Summer trip" })).toBeDefined();
  });

  it("404 では理由を出したうえで動画を取り直させる", async () => {
    const user = userEvent.setup();
    const onStale = vi.fn();
    fetchMock.mockResolvedValueOnce(
      json({ code: "not_found", reason: "video_not_found", message: "not found" }, 404),
    );
    renderTitle({ initial: video, onStale });
    await user.click(editButton());
    await user.type(nameInput(), "Summer trip{Enter}");
    expect((await screen.findByRole("alert")).textContent).toBe(
      "Couldn't save the name: The video wasn't found.",
    );
    expect(onStale).toHaveBeenCalledTimes(1);
  });

  it("編集中に動画が取り直されても、入力の文字を書き換えない（Edge Case「同時に変更」）", async () => {
    const user = userEvent.setup();
    renderTitle({ initial: video });
    await user.click(editButton());
    await user.clear(nameInput());
    await user.type(nameInput(), "Mine");
    act(() => harness.set({ ...video, title: "Theirs", displayName: "Theirs" }));
    expect(nameInput().value).toBe("Mine");
    await user.keyboard("{Escape}");
    expect(screen.getByRole("heading", { level: 1, name: "Theirs" })).toBeDefined();
  });
});
