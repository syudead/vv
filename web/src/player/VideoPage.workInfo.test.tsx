import { act, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useEffect, useState } from "react";
import { MemoryRouter, Route, Routes } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { setRenderedAudience as forgetAudience, type Video } from "../api/client";
import { emitServerEvent, installFakeEventSource } from "../api/fakeEventSource";
import type { WorkInfo } from "../api/workInfo";
import { type Audience, AudienceProvider } from "../auth/audience";
import { ToastProvider } from "../ui/Toast";
import { TooltipProvider } from "../ui/shadcn/tooltip";
import VideoDetailsEditor from "./detailsEditor/VideoDetailsEditor";
import type { PlayerControls } from "./playerControls";
import VideoPage from "./VideoPage";

// 動画ページの作品情報の行と、そこから開く全画面の編集画面。

vi.mock("./VideoPlayer", () => ({
  default: function VideoPlayerMock(props: {
    onControls: (controls: PlayerControls | null) => void;
  }) {
    const [initial] = useState(props);
    useEffect(() => () => initial.onControls(null), [initial]);
    return <div data-testid="video-player" />;
  },
  canStartPlayback: () => true,
  initialPlayerStatus: {
    loading: false,
    reconnecting: false,
    playing: false,
    userActive: true,
    ended: false,
    stalled: false,
    positioned: false,
  },
}));

const info: WorkInfo = {
  code: "AB-123",
  dates: [{ kind: "release", date: "2024-03-01" }],
  people: [{ role: "performer", person: { id: 1, name: "Alice" } }],
  organizations: [],
  urls: [],
  externalIds: [],
};

const plain: Video = {
  id: 7,
  title: "テスト動画",
  public: false,
  sizeBytes: 1_000,
  addedAt: "2026-09-01T00:00:00Z",
  updatedAt: "2026-09-01T00:00:00Z",
  fileCreatedAt: "2026-09-01T00:00:00Z",
  playable: true,
  probeState: "done",
  thumbnailState: "done",
  thumbnailUrl: "/api/videos/7/thumbnail",
  previewState: "done",
  durationMs: 242_000,
  width: 1920,
  height: 1080,
  container: "mp4",
  videoCodec: "h264",
  audioCodec: "aac",
  location: { path: "/media/テスト動画.mp4", openable: false },
  tags: [],
};

const withInfo: Video = { ...plain, workInfo: info };

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function person(id: number, name: string) {
  return {
    id,
    name,
    aliases: [],
    externalIds: [],
    videoCount: 1,
    createdAt: "2026-09-01T00:00:00Z",
  };
}

/** server は経路ごとの応答を差し替えられる偽のサーバーである。 */
const server = {
  video: withInfo,
  details: vi.fn<(body: unknown) => Response>(),
  displayName: vi.fn<(body: unknown) => Response>(),
  people: vi.fn<(url: URL) => Response>(),
  createPerson: vi.fn<(body: unknown) => Response>(),
  requests: [] as string[],
};

function renderPage(audience: Audience = "owner") {
  return render(
    <TooltipProvider>
      <ToastProvider>
        <MemoryRouter initialEntries={["/videos/7"]}>
          <AudienceProvider audience={audience}>
            <Routes>
              <Route path="/videos/:id" element={<VideoPage />} />
              <Route path="/videos/:id/edit" element={<VideoDetailsEditor />} />
            </Routes>
          </AudienceProvider>
        </MemoryRouter>
      </ToastProvider>
    </TooltipProvider>,
  );
}

async function openPage(audience: Audience = "owner") {
  renderPage(audience);
  await screen.findByRole("heading", { level: 1, name: /テスト動画|New title/ });
}

function workDetails() {
  return screen.getByRole("list", { name: "Work details" });
}

async function openEditor() {
  const user = userEvent.setup();
  await user.click(screen.getByRole("link", { name: "Edit details" }));
  await screen.findByRole("heading", { level: 1, name: "Edit details" });
  return user;
}

function field(name: string) {
  return screen.getByRole<HTMLInputElement>("textbox", { name });
}

function puts() {
  return server.requests.filter((r) => r === "PUT /api/videos/7/details");
}

describe("VideoPage work information", () => {
  const fetchMock = vi.fn<typeof fetch>();

  beforeEach(() => {
    server.video = withInfo;
    server.details.mockReset();
    server.displayName.mockReset();
    server.people.mockReset();
    server.people.mockImplementation(() => json({ items: [], total: 0, totalAll: 0 }));
    server.createPerson.mockReset();
    server.requests = [];
    installFakeEventSource();
    forgetAudience(null);
    fetchMock.mockReset();
    fetchMock.mockImplementation((input, init) => {
      const url = String(input);
      const method = init?.method ?? "GET";
      server.requests.push(`${method} ${url.split("?")[0]!}`);
      const body: unknown =
        init?.body === undefined ? undefined : JSON.parse(String(init.body));
      if (url === "/api/auth/session") return Promise.resolve(json({ state: "owner" }));
      if (url === "/api/thumbnail-edits/readiness")
        return Promise.resolve(json({ connection: true }));
      if (url === "/api/videos/7/details" && method === "PUT")
        return Promise.resolve(server.details(body));
      if (url === "/api/videos/7/display-name" && method === "PUT")
        return Promise.resolve(server.displayName(body));
      if (url.startsWith("/api/people") && method === "GET")
        return Promise.resolve(server.people(new URL(url, "http://localhost")));
      if (url === "/api/people" && method === "POST")
        return Promise.resolve(server.createPerson(body));
      if (url === "/api/videos/7") return Promise.resolve(json(server.video));
      if (url === "/api/videos/7/related") return Promise.resolve(json({ items: [] }));
      if (url === "/api/videos/7/subtitles")
        return Promise.resolve(json({ subtitles: [] }));
      if (url === "/api/tags") return Promise.resolve(json([]));
      return Promise.resolve(json({}));
    });
    vi.stubGlobal("fetch", fetchMock);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  it("値のある項目だけを情報欄に出し、無い項目は出さない", async () => {
    await openPage();
    const row = workDetails();
    expect(
      within(row)
        .getAllByRole("listitem")
        .map((item) => item.textContent),
    ).toEqual(["CodeAB-123", "Released2024-03-01", "PerformerAlice"]);
    expect(within(row).queryByText("Recorded")).toBeNull();
    expect(within(row).queryByText("Links")).toBeNull();
    // 行の順: ファイルの行、作品情報の行、技術の行。
    const facts = screen.getByRole("list", { name: "File details" });
    const technical = screen.getByRole("list", { name: "Technical details" });
    expect(
      facts.compareDocumentPosition(row) & Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();
    expect(
      row.compareDocumentPosition(technical) & Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();
  });

  it("作品情報の無い動画は、情報欄を今までどおりに描く", async () => {
    server.video = plain;
    await openPage();
    expect(screen.queryByRole("list", { name: "Work details" })).toBeNull();
    expect(screen.getByRole("list", { name: "File details" })).toBeDefined();
    expect(screen.getByRole("list", { name: "Technical details" })).toBeDefined();
  });

  it("4 人以上の役割は 3 人と +{n} more を出し、押すと全員を出して 4 人目へフォーカスを移す", async () => {
    server.video = {
      ...plain,
      workInfo: {
        ...info,
        people: ["A", "B", "C", "D", "E"].map((name, index) => ({
          role: "performer" as const,
          person: { id: index + 1, name },
        })),
      },
    };
    await openPage();
    const more = within(workDetails()).getByRole("button", {
      name: "Show all 5 performers",
    });
    expect(more.textContent).toBe("+2 more");
    expect(within(workDetails()).queryByText("D")).toBeNull();
    await userEvent.setup().click(more);
    const fourth = within(workDetails()).getByText("D");
    expect(document.activeElement).toBe(fourth);
    expect(within(workDetails()).getByText("E")).toBeDefined();
    expect(
      within(workDetails()).getByRole("button", { name: "Show fewer" }),
    ).toBeDefined();
  });

  it("ゲストには項目も編集の入口も無い", async () => {
    server.video = { ...plain, location: undefined, public: true };
    await openPage("guest");
    expect(screen.queryByRole("list", { name: "Work details" })).toBeNull();
    expect(screen.queryByRole("link", { name: "Edit details" })).toBeNull();
  });

  it("ページから開いた編集画面に、今の表示名と項目が出る", async () => {
    server.video = { ...withInfo, displayName: "表示名", fileTitle: "file-title" };
    await openPage();
    await openEditor();
    expect(field("Name").value).toBe("表示名");
    expect(field("Name").placeholder).toBe("file-title");
    expect(document.activeElement).toBe(field("Name"));
    expect(field("Code").value).toBe("AB-123");
    expect(field("Released").value).toBe("2024-03-01");
    expect(screen.getByRole("button", { name: "Remove Alice" })).toBeDefined();
    // 入口から開いたときは、ページの Video から始めて読み直さない。
    expect(server.requests.filter((r) => r === "GET /api/videos/7")).toHaveLength(1);
  });

  it("表示名・品番・発売日・出演者 2 人（選ぶ 1 人、作る 1 人）を 1 回の PUT で保存し、ページが応答を示す", async () => {
    server.video = plain;
    server.people.mockImplementation((url) => {
      const q = url.searchParams.get("q") ?? "";
      return json({
        items: "Bob".startsWith(q) ? [person(2, "Bob")] : [],
        total: 0,
        totalAll: 0,
      });
    });
    server.createPerson.mockImplementation(() => json(person(3, "Carol"), 201));
    const saved: Video = {
      ...plain,
      title: "New title",
      displayName: "New title",
      fileTitle: "テスト動画",
      workInfo: {
        code: "XY-9",
        dates: [{ kind: "release", date: "2025" }],
        people: [
          { role: "performer", person: { id: 2, name: "Bob" } },
          { role: "performer", person: { id: 3, name: "Carol" } },
        ],
        organizations: [],
        urls: [],
        externalIds: [],
      },
    };
    server.details.mockImplementation(() => json(saved));
    await openPage();
    const user = await openEditor();

    // 空の欄には貼り付けて 1 回で入れる。1 文字ずつ打つと編集画面を打鍵ごとに描き直し、
    // CI の遅い走りで 5 秒の上限に近づく。送る値はどちらでも同じ。
    for (const [name, value] of [
      ["Name", "New title"],
      ["Code", "XY-9"],
      ["Released", "2025"],
    ] as const) {
      await user.click(field(name));
      await user.paste(value);
    }
    const picker = screen.getByRole("combobox", { name: "Add a performer" });
    await user.type(picker, "Bob");
    await user.click(await screen.findByRole("option", { name: /^Bob/ }));
    await user.type(picker, "Carol");
    await user.keyboard("{Enter}");
    expect(await screen.findByText("New")).toBeDefined();
    expect(server.createPerson).not.toHaveBeenCalled();

    server.video = saved;
    await user.click(screen.getByRole("button", { name: "Save" }));

    expect(
      await screen.findByRole("heading", { level: 1, name: "New title" }),
    ).toBeDefined();
    expect(server.createPerson).toHaveBeenCalledTimes(1);
    expect(server.createPerson).toHaveBeenCalledWith({ name: "Carol" });
    expect(puts()).toHaveLength(1);
    expect(server.details).toHaveBeenCalledWith({
      displayName: "New title",
      workInfo: {
        code: "XY-9",
        description: null,
        dates: [{ kind: "release", date: "2025" }],
        people: [
          { role: "performer", person: { id: 2 } },
          { role: "performer", person: { id: 3 } },
        ],
        organizations: [],
        urls: [],
        externalIds: [],
      },
    });
    expect(
      within(workDetails())
        .getAllByRole("listitem")
        .map((item) => item.textContent),
    ).toEqual(["CodeXY-9", "Released2025", "PerformersBob, Carol"]);
    await waitFor(() =>
      expect(document.activeElement).toBe(
        screen.getByRole("link", { name: "Edit details" }),
      ),
    );
  });

  it("編集画面で項目を消して保存すると、情報欄からその項目が消える", async () => {
    const cleared: Video = { ...plain, workInfo: { ...info, code: undefined } };
    server.details.mockImplementation(() => json(cleared));
    await openPage();
    const user = await openEditor();
    await user.clear(field("Code"));
    server.video = cleared;
    await user.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() =>
      expect(
        screen.queryByRole("heading", { level: 1, name: "Edit details" }),
      ).toBeNull(),
    );
    expect(server.details.mock.calls[0]![0]).toMatchObject({
      workInfo: { code: null },
    });
    expect(within(workDetails()).queryByText("Code")).toBeNull();
    expect(within(workDetails()).getByText("2024-03-01")).toBeDefined();
  });

  it("dates[0].date の 400 では編集画面が開いたまま値を残し、その項目に理由を出し、離れても題名は変わらない", async () => {
    server.video = plain;
    server.details.mockImplementation(() =>
      json(
        {
          code: "invalid_request",
          message: "invalid",
          reason: "work_info_invalid_date",
          field: "dates[0].date",
        },
        400,
      ),
    );
    await openPage();
    const user = await openEditor();
    await user.type(field("Name"), "Renamed");
    await user.type(field("Recorded"), "2024-13");
    await user.click(screen.getByRole("button", { name: "Save" }));

    const recorded = field("Recorded");
    await waitFor(() => expect(recorded.getAttribute("aria-invalid")).toBe("true"));
    expect(
      screen.getByText("Enter a date as YYYY, YYYY-MM or YYYY-MM-DD."),
    ).toBeDefined();
    expect(document.activeElement).toBe(recorded);
    expect(recorded.value).toBe("2024-13");
    expect(field("Name").value).toBe("Renamed");

    await user.click(screen.getByRole("button", { name: "Cancel" }));
    const dialog = await screen.findByRole("alertdialog", { name: "Discard changes?" });
    await user.click(within(dialog).getByRole("button", { name: "Discard" }));
    expect(
      await screen.findByRole("heading", { level: 1, name: "テスト動画" }),
    ).toBeDefined();
  });

  it("displayName の 400 では表示名の下に理由を出す", async () => {
    server.details.mockImplementation(() =>
      json(
        {
          code: "invalid_request",
          message: "invalid",
          reason: "display_name_control_characters",
          field: "displayName",
        },
        400,
      ),
    );
    await openPage();
    const user = await openEditor();
    await user.type(field("Name"), "Bad");
    await user.keyboard("{Enter}");
    await waitFor(() => expect(field("Name").getAttribute("aria-invalid")).toBe("true"));
    expect(screen.getByText("The name can't contain control characters.")).toBeDefined();
  });

  it("何も変えずに Cancel を押すと、確かめずにページへ戻る", async () => {
    await openPage();
    const user = await openEditor();
    await user.click(screen.getByRole("button", { name: "Cancel" }));
    expect(
      await screen.findByRole("heading", { level: 1, name: "テスト動画" }),
    ).toBeDefined();
    expect(screen.queryByRole("alertdialog")).toBeNull();
    expect(server.details).not.toHaveBeenCalled();
  });

  it("ページの題名のその場編集は今も PUT /api/videos/{id}/display-name を送り、作品情報の行を残す", async () => {
    // display-name の応答は契約上 workInfo を載せない。
    server.displayName.mockImplementation(() =>
      json({ ...plain, title: "Quick", displayName: "Quick" }),
    );
    await openPage();
    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: "Edit name" }));
    const input = screen.getByRole("textbox", { name: "Display name" });
    await user.clear(input);
    await user.type(input, "Quick");
    await user.keyboard("{Enter}");
    await waitFor(() =>
      expect(server.displayName).toHaveBeenCalledWith({ displayName: "Quick" }),
    );
    expect(await screen.findByRole("heading", { level: 1, name: "Quick" })).toBeDefined();
    expect(within(workDetails()).getByText("AB-123")).toBeDefined();
    expect(puts()).toHaveLength(0);
  });

  it("video の知らせでページを読み直し、ほかで書いた値を出す", async () => {
    await openPage();
    expect(within(workDetails()).getByText("AB-123")).toBeDefined();
    server.video = { ...withInfo, workInfo: { ...info, code: "CD-456" } };
    await act(async () => {
      await emitServerEvent("video", { id: 7 });
    });
    await waitFor(() => expect(within(workDetails()).getByText("CD-456")).toBeDefined());
  });
});
