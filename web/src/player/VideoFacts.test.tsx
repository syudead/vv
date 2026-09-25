import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { Video } from "../api/client";
import { TooltipProvider } from "../ui/Tooltip";
import { formatCodec, technicalSummary } from "./properties";
import VideoFacts from "./VideoFacts";

function json(body: unknown, status: number): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

const video: Video = {
  id: 7,
  title: "テスト動画",
  sizeBytes: 84_331_821,
  addedAt: "2026-09-20T03:00:00Z",
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
  progress: { positionMs: 1, completed: false, updatedAt: "2026-09-21T05:06:00Z" },
  tags: [],
  public: false,
};

function renderFacts(value: Video) {
  return render(
    <TooltipProvider>
      <VideoFacts video={value} />
    </TooltipProvider>,
  );
}

function listText(name: string): string[] {
  return within(screen.getByRole("list", { name }))
    .getAllByRole("listitem")
    .map((item) => item.textContent ?? "");
}

describe("VideoFacts", () => {
  const fetchMock = vi.fn<typeof fetch>();
  beforeEach(() => vi.stubGlobal("fetch", fetchMock));
  afterEach(() => {
    vi.unstubAllGlobals();
    fetchMock.mockReset();
  });

  it("ファイルの情報は長さ・サイズ・追加日の順で、最後に見た日時は出さない", () => {
    renderFacts(video);
    expect(listText("ファイルの情報")).toEqual([
      "長さ 4:02",
      "サイズ 80.4 MB",
      "追加日 2026/09/20",
    ]);
    expect(document.body.textContent).not.toContain("2026/09/21");
  });

  it("技術情報は解像度・コンテナ・映像・音声を大文字の表記で並べ、分からない値は省く", () => {
    const view = renderFacts(video);
    expect(listText("技術情報")).toEqual(["1920×1080", "MKV", "H.264", "AAC"]);
    view.unmount();

    renderFacts({ ...video, audioCodec: undefined });
    expect(listText("技術情報")).toEqual(["1920×1080", "MKV", "H.264"]);
  });

  it("読み取り前は長さを出さず、技術情報は読み取り中、失敗なら警告を 1 行出す", () => {
    const pending = {
      ...video,
      probeState: "pending" as const,
      durationMs: undefined,
      width: undefined,
      height: undefined,
      container: undefined,
      videoCodec: undefined,
      audioCodec: undefined,
    };
    const view = renderFacts(pending);
    expect(listText("ファイルの情報")).toEqual(["サイズ 80.4 MB", "追加日 2026/09/20"]);
    expect(screen.getByText("技術情報を読み取り中")).toBeDefined();
    view.unmount();

    renderFacts({ ...pending, probeState: "failed", probeError: "moov atom" });
    expect(screen.getByText("技術情報を読み取れませんでした").className).toContain(
      "text-warning",
    );
  });

  it("開けないときは「ファイルを開く」を出さず、コピーだけを置く", () => {
    renderFacts({ ...video, location: { path: "/media/a/b.mp4", openable: false } });
    expect(screen.queryByRole("button", { name: "ファイルを開く" })).toBeNull();
    expect(screen.getByRole("button", { name: "パスをコピー" })).toBeDefined();
  });

  it("所在が無ければ操作を置かない", () => {
    renderFacts({ ...video, location: undefined });
    expect(screen.queryByRole("button")).toBeNull();
  });

  it("「パスをコピー」は所在の絶対パスをクリップボードへ書く", async () => {
    const writeText = vi.fn<(text: string) => Promise<void>>().mockResolvedValue();
    vi.stubGlobal("navigator", { ...navigator, clipboard: { writeText } });
    renderFacts(video);
    fireEvent.click(screen.getByRole("button", { name: "パスをコピー" }));
    await waitFor(() => expect(writeText).toHaveBeenCalledWith("/media/a/b.mp4"));
  });

  it("安全でない接続（clipboard が無い）では、選んだ文字をコピーする方法に切り替える", () => {
    vi.stubGlobal("navigator", { ...navigator, clipboard: undefined });
    let copied = "";
    const execCommand = vi.fn(() => {
      const field = document.querySelector("textarea");
      // 実際のブラウザでは select() で入力欄にフォーカスが移る。jsdom は移さないので模す。
      field?.focus();
      copied = field?.value ?? "";
      return true;
    });
    Object.defineProperty(document, "execCommand", {
      value: execCommand,
      configurable: true,
    });
    renderFacts(video);
    const button = screen.getByRole("button", { name: "パスをコピー" });
    button.focus();
    fireEvent.click(button);
    expect(execCommand).toHaveBeenCalledWith("copy");
    expect(copied).toBe("/media/a/b.mp4");
    expect(document.querySelector("textarea")).toBeNull();
    // 選ぶために入力欄へ移ったフォーカスを、押したボタンへ戻す。
    expect(document.activeElement).toBe(button);
  });

  it("「ファイルを開く」は開く要求を送る", async () => {
    fetchMock.mockResolvedValue(new Response(null, { status: 204 }));
    renderFacts(video);
    fireEvent.click(screen.getByRole("button", { name: "ファイルを開く" }));
    await waitFor(() =>
      expect(fetchMock).toHaveBeenCalledWith(
        "/api/videos/7/open",
        expect.objectContaining({ method: "POST" }),
      ),
    );
    expect(screen.queryByRole("alert")).toBeNull();
  });

  it("file_missing なら理由付き、それ以外なら短い文言を情報の行の下に出し、次の操作で消す", async () => {
    fetchMock
      .mockResolvedValueOnce(json({ code: "file_missing", message: "無い" }, 409))
      .mockResolvedValueOnce(json({ code: "internal", message: "失敗" }, 500))
      .mockReturnValueOnce(new Promise(() => undefined));
    renderFacts(video);
    const button = screen.getByRole("button", { name: "ファイルを開く" });
    fireEvent.click(button);
    expect((await screen.findByRole("alert")).textContent).toBe(
      "開けませんでした: ファイルが見つかりません",
    );
    fireEvent.click(button);
    await waitFor(() =>
      expect(screen.getByRole("alert").textContent).toBe("開けませんでした"),
    );
    fireEvent.click(button);
    expect(screen.queryByRole("alert")).toBeNull();
  });
});

describe("property helpers", () => {
  it("コーデックを大文字の表記にする", () => {
    expect(formatCodec("h264")).toBe("H.264");
    expect(formatCodec("hevc")).toBe("H.265");
    expect(formatCodec("vp9")).toBe("VP9");
    expect(formatCodec("aac")).toBe("AAC");
  });

  it("技術情報の状態を返す", () => {
    expect(technicalSummary({ ...video, probeState: "pending" })).toEqual({
      kind: "pending",
    });
    expect(technicalSummary({ ...video, probeState: "failed" })).toEqual({
      kind: "failed",
    });
  });
});
