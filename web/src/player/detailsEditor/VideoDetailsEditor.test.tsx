import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes, useLocation, useNavigate } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { setRenderedAudience as forgetAudience, type Video } from "../../api/client";
import { installFakeEventSource } from "../../api/fakeEventSource";
import { AudienceProvider } from "../../auth/audience";
import { ToastProvider } from "../../ui/Toast";
import { TooltipProvider } from "../../ui/shadcn/tooltip";
import VideoDetailsEditor from "./VideoDetailsEditor";

// Direct editor routing and leaving; player/VideoPage.workInfo.test.tsx covers entry from the video page.

const video: Video = {
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
  previewState: "done",
  durationMs: 242_000,
  width: 1920,
  height: 1080,
  location: { path: "/media/movies/テスト動画.mp4", openable: false },
  tags: [],
  workInfo: {
    dates: [],
    people: [
      {
        role: "performer",
        person: { id: 1, name: "Alice", provisional: false },
        provisional: false,
      },
    ],
    organizations: [],
    urls: [],
    externalIds: [],
  },
};

const video8: Video = { ...video, id: 8, title: "別の動画", displayName: "Eight" };

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function person(id: number, name: string, aliases: string[] = []) {
  return {
    id,
    name,
    aliases: aliases.map((alias) => ({ name: alias })),
    externalIds: [],
    videoCount: 2,
    createdAt: "2026-09-01T00:00:00Z",
  };
}

const server = {
  video: (): Response => json(video),
  people: vi.fn<(url: URL) => Response>(),
  details: vi.fn<(body: unknown) => Response>(),
  createPerson: vi.fn<(body: unknown) => Response>(),
};

function Where() {
  const location = useLocation();
  return <p data-testid="where">{location.pathname}</p>;
}

function Back() {
  const navigate = useNavigate();
  return (
    <>
      <button type="button" onClick={() => void navigate(-1)}>
        Browser back
      </button>
      <button type="button" onClick={() => void navigate(1)}>
        Browser forward
      </button>
      <button type="button" onClick={() => void navigate("/videos/8/edit")}>
        Open video 8
      </button>
    </>
  );
}

function renderEditor() {
  return render(
    <TooltipProvider>
      <ToastProvider>
        <MemoryRouter initialEntries={["/", "/videos/7/edit"]} initialIndex={1}>
          <AudienceProvider audience="owner">
            <Where />
            <Back />
            <Routes>
              <Route path="/videos/:id/edit" element={<VideoDetailsEditor />} />
              <Route path="/videos/:id" element={<p>video page</p>} />
              <Route path="/" element={<p>library</p>} />
            </Routes>
          </AudienceProvider>
        </MemoryRouter>
      </ToastProvider>
    </TooltipProvider>,
  );
}

function where() {
  return screen.getByTestId("where").textContent;
}

describe("VideoDetailsEditor", () => {
  const fetchMock = vi.fn<typeof fetch>();

  beforeEach(() => {
    server.video = () => json(video);
    server.people.mockReset();
    server.people.mockImplementation(() => json({ items: [], total: 0, totalAll: 0 }));
    server.details.mockReset();
    server.details.mockImplementation(() => json(video));
    server.createPerson.mockReset();
    installFakeEventSource();
    forgetAudience(null);
    fetchMock.mockReset();
    fetchMock.mockImplementation((input, init) => {
      const url = String(input);
      const method = init?.method ?? "GET";
      const body: unknown =
        init?.body === undefined ? undefined : JSON.parse(String(init.body));
      if (url === "/api/videos/7" && method === "GET")
        return Promise.resolve(server.video());
      if (url === "/api/videos/8" && method === "GET")
        return Promise.resolve(json(video8));
      if (url === "/api/videos/8/details") return Promise.resolve(json(video8));
      if (url.startsWith("/api/people") && method === "GET")
        return Promise.resolve(server.people(new URL(url, "http://localhost")));
      if (url === "/api/people" && method === "POST")
        return Promise.resolve(server.createPerson(body));
      if (url === "/api/videos/7/details") return Promise.resolve(server.details(body));
      return Promise.resolve(json({}));
    });
    vi.stubGlobal("fetch", fetchMock);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  async function opened() {
    renderEditor();
    await screen.findByRole("textbox", { name: "Name" });
  }

  it("URL で開くと動画を読み、左の列にファイル名と事実を出す", async () => {
    await opened();
    expect(fetchMock.mock.calls.map(([input]) => String(input))).toContain(
      "/api/videos/7",
    );
    const aside = screen.getByRole("complementary", { name: "Video" });
    expect(within(aside).getByText("テスト動画.mp4")).toBeDefined();
    expect(within(aside).getByText("4:02 · 1000 B · 1920×1080")).toBeDefined();
    expect(document.title).toBe("Edit details · テスト動画");
  });

  it("動画が無ければ消えた状態を出し、Save を押せない", async () => {
    server.video = () =>
      json({ code: "not_found", message: "x", reason: "video_not_found" }, 404);
    renderEditor();
    expect(await screen.findByText("This video no longer exists.")).toBeDefined();
    expect(
      screen.getByRole("button", { name: "Save" }).getAttribute("aria-disabled"),
    ).toBe("true");
    expect(screen.queryByRole("button", { name: "Retry" })).toBeNull();
  });

  it("読めなければ理由と Retry を出し、Retry で読み直す", async () => {
    server.video = () => json({ code: "internal", message: "boom" }, 500);
    renderEditor();
    expect(await screen.findByText(/Couldn't load the video:/)).toBeDefined();
    server.video = () => json(video);
    await userEvent.setup().click(screen.getByRole("button", { name: "Retry" }));
    expect(await screen.findByRole("textbox", { name: "Name" })).toBeDefined();
  });

  it("候補は別名で当たった行に Alias を添え、一致する人がいても作成の行を出す", async () => {
    server.people.mockImplementation(() =>
      json({
        items: [person(2, "Bob"), person(4, "Robert", ["bobby"])],
        total: 2,
        totalAll: 2,
      }),
    );
    await opened();
    const user = userEvent.setup();
    await user.type(screen.getByRole("combobox", { name: "Add a director" }), "bob");
    const options = await screen.findAllByRole("option");
    expect(options.map((option) => option.textContent)).toEqual([
      "Bob2 videos",
      "RobertAlias: bobby2 videos",
      'Create "bob"',
    ]);
    const url = server.people.mock.calls.at(-1)![0];
    expect(url.searchParams.get("limit")).toBe("8");
    expect(url.searchParams.get("q")).toBe("bob");
  });

  it("8 件の外に完全一致がありうる Enter は limit なしで読み直し、一致があれば選ぶ", async () => {
    server.people.mockImplementation((url) =>
      url.searchParams.get("limit") === null
        ? json({ items: [person(9, "Dan")], total: 1, totalAll: 30 })
        : json({
            items: [person(10, "Danny")],
            total: 30,
            totalAll: 30,
            nextCursor: "c",
          }),
    );
    await opened();
    const user = userEvent.setup();
    const picker = screen.getByRole("combobox", { name: "Add a performer" });
    await user.type(picker, "Dan");
    await screen.findByRole("option", { name: /^Danny/ });
    await user.keyboard("{Enter}");
    expect(await screen.findByRole("button", { name: "Remove Dan" })).toBeDefined();
    expect(screen.queryByText("New")).toBeNull();
  });

  it("まだ作っていない名前は別の役割で New の行として出し、保存で 1 度だけ作る", async () => {
    server.createPerson.mockImplementation(() => json(person(5, "Eve"), 201));
    await opened();
    const user = userEvent.setup();
    await user.type(screen.getByRole("combobox", { name: "Add a performer" }), "Eve");
    await user.click(await screen.findByRole("option", { name: 'Create "Eve"' }));
    await user.type(screen.getByRole("combobox", { name: "Add a director" }), "Eve");
    const unsaved = await screen.findByRole("option", { name: "Eve, new" });
    expect(screen.queryByRole("option", { name: 'Create "Eve"' })).toBeNull();
    await user.click(unsaved);
    await user.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(server.details).toHaveBeenCalledTimes(1));
    expect(server.createPerson).toHaveBeenCalledTimes(1);
    expect(server.details.mock.calls[0]![0]).toMatchObject({
      workInfo: {
        people: [
          { role: "performer", person: { id: 1 } },
          { role: "performer", person: { id: 5 } },
          { role: "director", person: { id: 5 } },
        ],
      },
    });
  });

  it("作れた後に PUT が失敗したら数を添えて出し、もう一度の Save は PUT だけを送る", async () => {
    server.createPerson.mockImplementation(() => json(person(5, "Eve"), 201));
    server.details.mockImplementation(() =>
      json({ code: "internal", message: "x" }, 500),
    );
    await opened();
    const user = userEvent.setup();
    await user.type(screen.getByRole("combobox", { name: "Add a performer" }), "Eve");
    await user.click(await screen.findByRole("option", { name: 'Create "Eve"' }));
    await user.click(screen.getByRole("button", { name: "Save" }));
    expect(
      await screen.findByText(/^1 person was created, but the details weren't saved:/),
    ).toBeDefined();
    expect(screen.queryByText("New")).toBeNull();
    await user.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(server.details).toHaveBeenCalledTimes(2));
    expect(server.createPerson).toHaveBeenCalledTimes(1);
  });

  it("入力の無い経路の誤りは、失敗の行に経路と一緒に出す", async () => {
    server.details.mockImplementation(() =>
      json(
        {
          code: "invalid_request",
          message: "x",
          reason: "work_info_too_many",
          limit: 100,
          field: "people",
        },
        400,
      ),
    );
    await opened();
    await userEvent.setup().click(screen.getByRole("button", { name: "Save" }));
    const line = await screen.findByRole("alert");
    expect(line.textContent).toBe(
      "Couldn't save the details: Too many: up to 100. people",
    );
  });

  it("ブラウザの Back は、変えていなければそのまま離れ、変えていれば確かめる", async () => {
    await opened();
    const user = userEvent.setup();
    await user.type(screen.getByRole("textbox", { name: "Code" }), "X");
    await user.click(screen.getByRole("button", { name: "Browser back" }));
    const dialog = await screen.findByRole("alertdialog", { name: "Discard changes?" });
    expect(where()).toBe("/videos/7/edit");
    await user.click(within(dialog).getByRole("button", { name: "Keep editing" }));
    expect(screen.getByRole<HTMLInputElement>("textbox", { name: "Code" }).value).toBe(
      "X",
    );

    await user.click(screen.getByRole("button", { name: "Browser back" }));
    const again = await screen.findByRole("alertdialog", { name: "Discard changes?" });
    await user.click(within(again).getByRole("button", { name: "Discard" }));
    await waitFor(() => expect(where()).toBe("/videos/7"));
    expect(server.details).not.toHaveBeenCalled();
    await user.click(screen.getByRole("button", { name: "Browser back" }));
    expect(where()).toBe("/");
  });

  it("URL で開いた編集画面を Cancel で離れると、動画ページの Back は前のページへ戻る", async () => {
    await opened();
    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: "Cancel" }));
    await waitFor(() => expect(where()).toBe("/videos/7"));
    await user.click(screen.getByRole("button", { name: "Browser back" }));
    expect(where()).toBe("/");
    expect(screen.getByText("library")).toBeDefined();
  });

  it("URL で開いた編集画面を Save で離れると、動画ページの Back は前のページへ戻る", async () => {
    await opened();
    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(where()).toBe("/videos/7"));
    expect(server.details).toHaveBeenCalledTimes(1);
    await user.click(screen.getByRole("button", { name: "Browser back" }));
    expect(where()).toBe("/");
    await user.click(screen.getByRole("button", { name: "Browser forward" }));
    expect(where()).toBe("/videos/7");
  });

  it("経路の id が変わると、その動画を読み直して保存する", async () => {
    await opened();
    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: "Open video 8" }));
    await waitFor(() =>
      expect(screen.getByRole<HTMLInputElement>("textbox", { name: "Name" }).value).toBe(
        "Eight",
      ),
    );
    await user.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() =>
      expect(fetchMock.mock.calls.map(([input]) => String(input))).toContain(
        "/api/videos/8/details",
      ),
    );
    expect(server.details).not.toHaveBeenCalled();
  });

  it("Enter の後の読み直しの間に綴りを変えても、新しい候補を選べる", async () => {
    server.people.mockImplementation((url) =>
      url.searchParams.get("q") === "Dana"
        ? json({ items: [person(11, "Dana")], total: 1, totalAll: 1 })
        : json({ items: [person(10, "Danny")], total: 9, totalAll: 9, nextCursor: "c" }),
    );
    const answer = fetchMock.getMockImplementation()!;
    fetchMock.mockImplementation((input, init) => {
      const url = new URL(String(input), "http://localhost");
      if (url.pathname !== "/api/people" || url.searchParams.has("limit"))
        return answer(input, init);
      return new Promise<Response>((_, reject) => {
        init?.signal?.addEventListener("abort", () =>
          reject(new DOMException("aborted", "AbortError")),
        );
      });
    });
    await opened();
    const user = userEvent.setup();
    const picker = screen.getByRole("combobox", { name: "Add a performer" });
    await user.type(picker, "Dan");
    await screen.findByRole("option", { name: /^Danny/ });
    await user.keyboard("{Enter}");
    await waitFor(() =>
      expect(
        fetchMock.mock.calls.some(([input]) => String(input) === "/api/people?q=Dan"),
      ).toBe(true),
    );
    await user.type(picker, "a");
    await user.click(await screen.findByRole("option", { name: /^Dana/ }));
    expect(await screen.findByRole("button", { name: "Remove Dana" })).toBeDefined();
  });
});
