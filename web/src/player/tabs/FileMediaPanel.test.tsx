import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { Scan, Video } from "../../api/client";
import { formatPreciseTime } from "../../i18n";
import { ScanProvider } from "../../shell/ScanProvider";
import { OwnerAudience } from "../../testing/audience";
import { ToastProvider } from "../../ui/Toast";
import { TooltipProvider } from "../../ui/shadcn/tooltip";
import FileMediaPanel from "./FileMediaPanel";

function json(body: unknown, status: number): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

// Noon keeps the formatted dates stable in any time zone.
const added = new Date(2026, 8, 20, 12, 0).toISOString();
const edited = new Date(2026, 8, 27, 15, 4).toISOString();
const created = new Date(2026, 6, 3, 9, 30).toISOString();
const modified = new Date(2026, 6, 4, 8, 15, 30, 250).toISOString();

const video: Video = {
  id: 7,
  virtual: false,
  title: "テスト動画",
  sizeBytes: 84_331_821,
  addedAt: added,
  updatedAt: edited,
  fileCreatedAt: created,
  fileModifiedAt: modified,
  playable: true,
  probeState: "done",
  thumbnailState: "done",
  previewState: "done",
  durationMs: 242_000,
  width: 1920,
  height: 1080,
  container: "mkv",
  videoCodec: "h264",
  audioCodec: "aac",
  location: { path: "/media/a/b.mp4", openable: true },
  tags: [],
  public: false,
};

function renderPanel(value: Video, owner = true) {
  return render(
    <TooltipProvider>
      <ToastProvider>
        <FileMediaPanel video={value} owner={owner} />
      </ToastProvider>
    </TooltipProvider>,
  );
}

function fileFacts(): string[] {
  return within(screen.getByLabelText("File"))
    .getAllByRole("term")
    .map(
      (term) => `${term.textContent ?? ""} ${term.nextElementSibling?.textContent ?? ""}`,
    );
}

describe("FileMediaPanel", () => {
  const fetchMock = vi.fn<typeof fetch>();
  beforeEach(() => vi.stubGlobal("fetch", fetchMock));
  afterEach(() => {
    vi.unstubAllGlobals();
    fetchMock.mockReset();
  });

  it("所在と Media の節を置き、File の列はサイズと 4 つの日時を秒まで、ミリ秒を持つ値はミリ秒まで出す", () => {
    renderPanel(video);
    expect(
      screen.getAllByRole("heading", { level: 2 }).map((h) => h.textContent),
    ).toEqual(["Location", "Media"]);
    expect(screen.getByText("/media/a/b.mp4")).toBeDefined();
    expect(fileFacts()).toEqual([
      "Size 80.4 MB",
      `Added Sep 20, 2026 ${formatPreciseTime(added)}`,
      `Edited in vv Sep 27, 2026 ${formatPreciseTime(edited)}`,
      `File created Jul 3, 2026 ${formatPreciseTime(created)}`,
      `File modified Jul 4, 2026 ${formatPreciseTime(modified)}`,
    ]);
    expect(formatPreciseTime(edited)).toMatch(/^3:04:00\s?PM$/);
    expect(formatPreciseTime(modified)).toMatch(/^8:15:30\.250\s?AM$/);
    const modifiedTime = screen.getByText(/8:15:30/).closest("time");
    expect(modifiedTime?.getAttribute("datetime")).toBe(modified);
  });

  it("開けないときは「ファイルを開く」を出さず、コピーを置く", () => {
    renderPanel({ ...video, location: { path: "/media/a/b.mp4", openable: false } });
    expect(screen.queryByRole("button", { name: "Open file" })).toBeNull();
    expect(screen.getByRole("button", { name: "Copy path" })).toBeDefined();
  });

  it("所在の無い動画とゲストには所在の節も操作も置かない", () => {
    const view = renderPanel({ ...video, location: undefined });
    expect(screen.queryByRole("region", { name: "Location" })).toBeNull();
    view.unmount();
    renderPanel(video, false);
    expect(screen.queryByRole("region", { name: "Location" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Copy path" })).toBeNull();
    expect(fileFacts()).toContain(
      `File modified Jul 4, 2026 ${formatPreciseTime(modified)}`,
    );
  });

  it("安全でない接続（clipboard が無い）では、選んだ文字をコピーする方法に切り替える", () => {
    vi.stubGlobal("navigator", { ...navigator, clipboard: undefined });
    let copied = "";
    const execCommand = vi.fn(() => {
      const field = document.querySelector("textarea");
      // jsdom does not focus inputs on select() as browsers do.
      field?.focus();
      copied = field?.value ?? "";
      return true;
    });
    Object.defineProperty(document, "execCommand", {
      value: execCommand,
      configurable: true,
    });
    renderPanel(video);
    const button = screen.getByRole("button", { name: "Copy path" });
    button.focus();
    fireEvent.click(button);
    expect(execCommand).toHaveBeenCalledWith("copy");
    expect(copied).toBe("/media/a/b.mp4");
    expect(document.querySelector("textarea")).toBeNull();
    expect(document.activeElement).toBe(button);
  });

  it("「ファイルを開く」は開く要求を送る", async () => {
    fetchMock.mockResolvedValue(new Response(null, { status: 204 }));
    renderPanel(video);
    fireEvent.click(screen.getByRole("button", { name: "Open file" }));
    await waitFor(() =>
      expect(fetchMock).toHaveBeenCalledWith(
        "/api/videos/7/open",
        expect.objectContaining({ method: "POST" }),
      ),
    );
    expect(screen.queryByRole("alert")).toBeNull();
  });

  it("開けなかった理由をパネルの末尾に出し、次の操作で消す", async () => {
    fetchMock
      .mockResolvedValueOnce(json({ code: "file_missing", message: "x" }, 409))
      .mockResolvedValueOnce(
        json({ code: "forbidden", reason: "open_not_local", message: "x" }, 403),
      )
      .mockReturnValueOnce(new Promise(() => undefined));
    const { container } = renderPanel(video);
    const button = screen.getByRole("button", { name: "Open file" });
    fireEvent.click(button);
    const alert = await screen.findByRole("alert");
    expect(alert.textContent).toBe("Couldn't open the file: The video file is missing.");
    expect(container.firstElementChild?.lastElementChild).toBe(alert);
    fireEvent.click(button);
    await waitFor(() =>
      expect(screen.getByRole("alert").textContent).toBe(
        "Couldn't open the file: Files can only be opened on the computer running vv.",
      ),
    );
    fireEvent.click(button);
    expect(screen.queryByRole("alert")).toBeNull();
  });
});

describe("FileMediaPanel rescan", () => {
  const fetchMock = vi.fn<typeof fetch>();
  beforeEach(() => vi.stubGlobal("fetch", fetchMock));
  afterEach(() => {
    vi.unstubAllGlobals();
    fetchMock.mockReset();
  });

  const started: Scan = {
    id: 5,
    origin: "manual",
    status: "finding",
    issues: { failed: 0, substituted: 0, revision: 0 },
    state: "running",
    rescan: { scope: "video", videoId: 7 },
  };

  function serve(current: Scan | null, rescan = () => json({}, 202)) {
    fetchMock.mockImplementation((input, init) => {
      const url = String(input);
      if (url === "/api/media-folders") return Promise.resolve(json([{}], 200));
      if (url === "/api/scans/current") {
        return Promise.resolve(current === null ? json({}, 404) : json(current, 200));
      }
      if (url === "/api/scans/rescan" && init?.method === "POST") {
        return Promise.resolve(rescan());
      }
      return Promise.resolve(json({}, 404));
    });
  }

  function renderOwnerPanel(value: Video = video) {
    return render(
      <TooltipProvider>
        <ToastProvider>
          <OwnerAudience>
            <ScanProvider>
              <FileMediaPanel video={value} owner />
            </ScanProvider>
          </OwnerAudience>
        </ToastProvider>
      </TooltipProvider>,
    );
  }

  const rescanCalls = () =>
    fetchMock.mock.calls.filter(([input]) => String(input) === "/api/scans/rescan");

  async function readyRescan() {
    const control = await screen.findByRole("button", { name: "Rescan" });
    await waitFor(() => expect(control.getAttribute("aria-disabled")).toBeNull());
    return control;
  }

  it("開けない所在でも「パスをコピー」と「Rescan」を置き、確かめずに動画の範囲を送る", async () => {
    serve(null, () => json(started, 202));
    renderOwnerPanel({ ...video, location: { path: "/media/a/b.mp4", openable: false } });
    expect(screen.getByRole("button", { name: "Copy path" })).toBeDefined();
    expect(screen.queryByRole("button", { name: "Open file" })).toBeNull();

    fireEvent.click(await readyRescan());
    await waitFor(() => expect(rescanCalls()).toHaveLength(1));
    expect(rescanCalls()[0]?.[1]?.body).toBe(
      JSON.stringify({ scope: "video", videoId: 7 }),
    );
    expect(screen.queryByRole("alertdialog")).toBeNull();
  });

  it("手動のスキャン中は理由を説明に持って押せず、何も送らない", async () => {
    serve({
      id: 3,
      origin: "manual",
      status: "running",
      issues: { failed: 0, substituted: 0, revision: 0 },
      state: "running",
    });
    renderOwnerPanel();
    const control = await screen.findByRole("button", {
      name: "Rescan",
      description: "A scan is running. You can start another when it finishes.",
    });
    expect(control.getAttribute("aria-disabled")).toBe("true");
    fireEvent.click(control);
    expect(rescanCalls()).toHaveLength(0);
  });

  it("断られた開始はパネルの失敗の行に出す", async () => {
    serve(null, () =>
      json({ code: "scan_in_progress", message: "A scan is in progress." }, 409),
    );
    renderOwnerPanel();
    fireEvent.click(await readyRescan());
    expect((await screen.findByRole("alert")).textContent).toBe(
      "Couldn't start the force rescan: A scan is in progress. Try again when it finishes.",
    );
  });

  it("応答を失った開始が現れたら、失敗を消して開始を知らせる", async () => {
    let posted = false;
    fetchMock.mockImplementation((input, init) => {
      const url = String(input);
      if (url === "/api/media-folders") return Promise.resolve(json([{}], 200));
      if (url === "/api/scans/rescan" && init?.method === "POST") {
        posted = true;
        return Promise.resolve(json({ code: "internal", message: "lost" }, 500));
      }
      if (url === "/api/scans/current" && posted) {
        return Promise.resolve(json(started, 200));
      }
      return Promise.resolve(json({}, 404));
    });
    renderOwnerPanel();
    fireEvent.click(await readyRescan());

    expect(await screen.findByText("Started a force rescan of this video")).toBeDefined();
    expect(screen.queryByText(/Couldn't start the force rescan/)).toBeNull();
  });
});
