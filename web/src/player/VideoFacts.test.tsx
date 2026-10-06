import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { Video } from "../api/client";
import { formatDateTime } from "../i18n";
import { TooltipProvider } from "../ui/shadcn/tooltip";
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
  updatedAt: "2026-09-20T03:00:00Z",
  fileCreatedAt: "2026-09-20T03:00:00Z",
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

  it("ファイルの情報は長さ・サイズ・追加日・更新日時・作成日時の順で、最後に見た日時は出さない", () => {
    renderFacts(video);
    expect(listText("File details")).toEqual([
      "Length 4:02",
      "Size 80.4 MB",
      "Added Sep 20, 2026",
      "Edited Sep 20, 2026",
      "Created Sep 20, 2026",
    ]);
    expect(document.body.textContent).not.toContain("2026/09/21");
  });

  it("技術情報は解像度・コンテナ・映像・音声を大文字の表記で並べ、分からない値は省く", () => {
    const view = renderFacts(video);
    expect(listText("Technical details")).toEqual(["1920×1080", "MKV", "H.264", "AAC"]);
    view.unmount();

    renderFacts({ ...video, audioCodec: undefined });
    expect(listText("Technical details")).toEqual(["1920×1080", "MKV", "H.264"]);
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
    expect(listText("File details")).toEqual([
      "Size 80.4 MB",
      "Added Sep 20, 2026",
      "Edited Sep 20, 2026",
      "Created Sep 20, 2026",
    ]);
    expect(screen.getByText("Reading technical details…")).toBeDefined();
    view.unmount();

    renderFacts({ ...pending, probeState: "failed", probeError: "moov atom" });
    expect(screen.getByText("Couldn't read the technical details").className).toContain(
      "text-warning",
    );
  });

  it("開けないときは「ファイルを開く」を出さず、コピーだけを置く", () => {
    renderFacts({ ...video, location: { path: "/media/a/b.mp4", openable: false } });
    expect(screen.queryByRole("button", { name: "Open file" })).toBeNull();
    expect(screen.getByRole("button", { name: "Copy path" })).toBeDefined();
  });

  it("所在が無ければ操作を置かない", () => {
    renderFacts({ ...video, location: undefined });
    expect(screen.queryByRole("button", { name: "Open file" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Copy path" })).toBeNull();
  });

  it("「パスをコピー」は所在の絶対パスをクリップボードへ書く", async () => {
    const writeText = vi.fn<(text: string) => Promise<void>>().mockResolvedValue();
    vi.stubGlobal("navigator", { ...navigator, clipboard: { writeText } });
    renderFacts(video);
    fireEvent.click(screen.getByRole("button", { name: "Copy path" }));
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
    const button = screen.getByRole("button", { name: "Copy path" });
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
    fireEvent.click(screen.getByRole("button", { name: "Open file" }));
    await waitFor(() =>
      expect(fetchMock).toHaveBeenCalledWith(
        "/api/videos/7/open",
        expect.objectContaining({ method: "POST" }),
      ),
    );
    expect(screen.queryByRole("alert")).toBeNull();
  });

  it("開けなかった理由を API エラーの英語の文で情報の行の下に出し、次の操作で消す", async () => {
    fetchMock
      .mockResolvedValueOnce(json({ code: "file_missing", message: "x" }, 409))
      .mockResolvedValueOnce(json({ code: "internal", message: "x" }, 500))
      .mockResolvedValueOnce(
        json({ code: "forbidden", reason: "open_not_local", message: "x" }, 403),
      )
      .mockReturnValueOnce(new Promise(() => undefined));
    renderFacts(video);
    const button = screen.getByRole("button", { name: "Open file" });
    fireEvent.click(button);
    expect((await screen.findByRole("alert")).textContent).toBe(
      "Couldn't open the file: The video file is missing.",
    );
    fireEvent.click(button);
    await waitFor(() =>
      expect(screen.getByRole("alert").textContent).toBe(
        "Couldn't open the file: Something went wrong on the server.",
      ),
    );
    fireEvent.click(button);
    await waitFor(() =>
      expect(screen.getByRole("alert").textContent).toBe(
        "Couldn't open the file: Files can only be opened on the computer running vv.",
      ),
    );
    fireEvent.click(button);
    expect(screen.queryByRole("alert")).toBeNull();
  });

  describe("日付の項目（specs/033-video-dates/ui-design.md「Video facts」）", () => {
    // 日時はブラウザのタイムゾーンで表すので、その日の正午を渡して日付が変わらないようにする。
    const added = new Date(2026, 8, 20, 12, 0).toISOString();
    const edited = new Date(2026, 8, 27, 15, 4).toISOString();
    const created = new Date(2026, 6, 3, 9, 30).toISOString();
    const dated: Video = {
      ...video,
      addedAt: added,
      updatedAt: edited,
      fileCreatedAt: created,
    };

    function dateButton(name: string | RegExp): HTMLElement {
      return within(screen.getByRole("list", { name: "File details" })).getByRole(
        "button",
        { name },
      );
    }

    it("3 つの日付は同じ日付だけの書式で、読み上げ名で区別でき、title に名前と時刻を持つ（要件 4、UI品質）", () => {
      renderFacts(dated);
      expect(listText("File details")).toEqual([
        "Length 4:02",
        "Size 80.4 MB",
        "Added Sep 20, 2026",
        "Edited Sep 27, 2026",
        "Created Jul 3, 2026",
      ]);
      expect(dateButton("Added Sep 20, 2026").title).toBe(
        `Added ${formatDateTime(added)}`,
      );
      expect(dateButton("Edited Sep 27, 2026").title).toBe(
        `Edited ${formatDateTime(edited)}`,
      );
      expect(dateButton("Created Jul 3, 2026").title).toBe(
        `Created ${formatDateTime(created)}`,
      );
      // 長さとサイズは引き金にしない。
      expect(
        within(screen.getByRole("list", { name: "File details" })).getAllByRole("button"),
      ).toHaveLength(3);
    });

    it("3 つの日付のアイコンはそれぞれ違う", () => {
      renderFacts(dated);
      const icons = ["Added", "Edited", "Created"].map(
        (name) =>
          dateButton(new RegExp(`^${name} `))
            .querySelector("svg")
            ?.getAttribute("class") ?? "",
      );
      expect(icons[0]).toContain("lucide-calendar-plus");
      expect(icons[1]).toContain("lucide-pencil-line");
      expect(icons[2]).toContain("lucide-file-clock");
    });

    it("押すと名前と日時の吹き出しを開き、Esc で閉じてフォーカスを引き金に戻す", async () => {
      renderFacts(dated);
      const trigger = dateButton("Edited Sep 27, 2026");
      trigger.focus();
      fireEvent.click(trigger);
      const bubble = await screen.findByRole("dialog");
      expect(bubble.textContent).toBe(`Edited ${formatDateTime(edited)}`);
      expect(trigger.getAttribute("aria-expanded")).toBe("true");

      fireEvent.keyDown(document.activeElement ?? document.body, { key: "Escape" });
      await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
      expect(document.activeElement).toBe(trigger);
    });

    it("ゲストの動画（所在が無い）でも更新日時と作成日時を出す", () => {
      renderFacts({ ...dated, location: undefined });
      expect(listText("File details")).toContain("Edited Sep 27, 2026");
      expect(listText("File details")).toContain("Created Jul 3, 2026");
    });
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
