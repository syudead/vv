import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes, useLocation } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { Person } from "../../api/credits";
import type { FaceAppearance, FaceCandidate } from "../../api/personFaces";
import { installFakeEventSource } from "../../api/fakeEventSource";
import { video } from "../../testing/groupPage";
import { ToastProvider } from "../../ui/Toast";
import { TooltipProvider } from "../../ui/shadcn/tooltip";
import PersonPage from "./PersonPage";
import PersonEditPage from "./edit/PersonEditPage";

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function person(overrides: Partial<Person> = {}): Person {
  return {
    id: 7,
    name: "Aoi",
    provisional: false,
    aliases: [],
    externalIds: [],
    profile: {},
    favorite: false,
    videoCount: 1,
    createdAt: "2026-01-01T00:00:00Z",
    ...overrides,
  };
}

interface Sent {
  method: string;
  url: URL;
  body: BodyInit | null | undefined;
}

const server = {
  person: null as Person | null,
  sent: [] as Sent[],
  /** When set, the face upload answers only once this settles. */
  uploadGate: null as Promise<void> | null,
  candidates: [] as FaceCandidate[],
  appearances: [] as FaceAppearance[],
  /** Served as the second page of appearances, after the cursor `page-2`. */
  laterAppearances: [] as FaceAppearance[],
  /** The person a confirm answers with, also served at its own address. */
  mergeTarget: null as Person | null,
};

async function respondLater(method: string, url: URL): Promise<Response> {
  if (url.pathname === "/api/people/7/image" && method === "PUT" && server.uploadGate) {
    // Answer with the person as it was when the upload was sent.
    const saved = { ...server.person, imageRevision: 3 } as Person;
    await server.uploadGate;
    return jsonResponse(saved);
  }
  return respond(method, url);
}

function respond(method: string, url: URL): Response {
  if (url.pathname === "/api/videos") {
    return jsonResponse({ items: [video(1)], total: 1 });
  }
  if (server.person === null) {
    return jsonResponse({ code: "not_found", reason: "person_not_found" }, 404);
  }
  if (url.pathname === "/api/people/7" && method === "GET")
    return jsonResponse(server.person);
  if (url.pathname === "/api/people/7/image" && method === "PUT") {
    server.person = { ...server.person, imageRevision: 3 };
    return jsonResponse(server.person);
  }
  if (url.pathname === "/api/people/7/face-candidates") {
    return jsonResponse({ items: server.candidates });
  }
  if (url.pathname === "/api/people/7/appearances") {
    if (url.searchParams.get("cursor") === "page-2") {
      return jsonResponse({ items: server.laterAppearances });
    }
    return jsonResponse({
      items: server.appearances,
      nextCursor: server.laterAppearances.length > 0 ? "page-2" : undefined,
    });
  }
  const split = /^\/api\/people\/7\/appearances\/(\d+)\/split$/.exec(url.pathname);
  if (split !== null && method === "POST") {
    server.appearances = server.appearances.filter(
      (item) => item.appearanceId !== Number(split[1]),
    );
    return jsonResponse(person({ id: 12, name: "", provisional: true }), 201);
  }
  const pair = /^\/api\/people\/7\/face-candidates\/(\d+)\/(confirm|dismiss)$/.exec(
    url.pathname,
  );
  if (pair !== null && method === "POST") {
    server.candidates = server.candidates.filter(
      (item) => item.person.id !== Number(pair[1]),
    );
    if (pair[2] === "dismiss") return new Response(null, { status: 204 });
    return jsonResponse(server.mergeTarget);
  }
  const target = server.mergeTarget;
  if (target !== null && url.pathname === `/api/people/${String(target.id)}`) {
    return jsonResponse(target);
  }
  if (url.pathname === "/api/favorites" && method === "PUT") {
    server.person = { ...server.person, favorite: true };
    return jsonResponse({ appliedVideos: 0, appliedGroups: 0, appliedSmartFolders: 0 });
  }
  return jsonResponse({ code: "not_found" }, 404);
}

class IdleIntersectionObserver {
  observe() {}
  disconnect() {}
}

beforeEach(() => {
  server.person = person();
  server.sent = [];
  server.uploadGate = null;
  server.candidates = [];
  server.appearances = [];
  server.laterAppearances = [];
  server.mergeTarget = null;
  vi.stubGlobal(
    "fetch",
    vi.fn<typeof fetch>((input, init) => {
      const url = new URL(String(input), "http://localhost");
      const method = init?.method ?? "GET";
      server.sent.push({ method, url, body: init?.body });
      return respondLater(method, url);
    }),
  );
  vi.stubGlobal("IntersectionObserver", IdleIntersectionObserver);
  installFakeEventSource();
});

afterEach(() => {
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

function VideoScreen() {
  const location = useLocation();
  const autoplay = (location.state as { autoplay?: boolean } | null)?.autoplay === true;
  return (
    <p data-testid="video-screen">
      {`${location.pathname}${location.search}${autoplay ? " autoplay" : ""}`}
    </p>
  );
}

function renderPage() {
  return render(
    <TooltipProvider>
      <ToastProvider>
        <MemoryRouter initialEntries={["/people/7"]}>
          <Routes>
            <Route path="/people/:id" element={<PersonPage />} />
            <Route path="/people/:id/edit" element={<PersonEditPage />} />
            <Route path="/videos/:id" element={<VideoScreen />} />
          </Routes>
        </MemoryRouter>
      </ToastProvider>
    </TooltipProvider>,
  );
}

function sent(method: string, pathname: string): Sent[] {
  return server.sent.filter(
    (request) => request.method === method && request.url.pathname === pathname,
  );
}

describe("PersonPage", () => {
  it("shows the uploaded face in place of the placeholder", async () => {
    const user = userEvent.setup();
    renderPage();
    await screen.findByRole("button", { name: "Add a face" });

    const file = new File(["face"], "face.png", { type: "image/png" });
    await user.upload(screen.getByTestId("person-face-input"), file);

    const image = await screen.findByRole("img", { name: "Aoi" });
    expect(image.getAttribute("src")).toBe("/api/people/7/image?v=3");
    const [upload] = sent("PUT", "/api/people/7/image");
    expect((upload?.body as FormData).get("image")).toBeInstanceOf(File);
    expect(screen.queryByRole("button", { name: "Add a face" })).toBeNull();
  });

  it("sends the person's favorite and shows it pressed after the response", async () => {
    const user = userEvent.setup();
    renderPage();
    await user.click(await screen.findByRole("button", { name: "Add to favorites" }));

    const pressed = await screen.findByRole("button", { name: "Remove from favorites" });
    expect(pressed.getAttribute("aria-pressed")).toBe("true");
    const [request] = sent("PUT", "/api/favorites");
    expect(JSON.parse(String(request?.body))).toEqual({
      videoIds: [],
      groupIds: [],
      personIds: [7],
      favorite: true,
    });
  });

  it("keeps a heart pressed during an upload when the older upload response arrives", async () => {
    let finishUpload = () => {};
    server.uploadGate = new Promise((resolve) => {
      finishUpload = resolve;
    });
    const user = userEvent.setup();
    renderPage();
    await screen.findByRole("button", { name: "Add a face" });

    const file = new File(["face"], "face.png", { type: "image/png" });
    await user.upload(screen.getByTestId("person-face-input"), file);
    await user.click(screen.getByRole("button", { name: "Add to favorites" }));
    await screen.findByRole("button", { name: "Remove from favorites" });
    finishUpload();

    const image = await screen.findByRole("img", { name: "Aoi" });
    expect(image.getAttribute("src")).toBe("/api/people/7/image?v=3");
    expect(
      screen
        .getByRole("button", { name: "Remove from favorites" })
        .getAttribute("aria-pressed"),
    ).toBe("true");
  });

  it("renews the age when the viewer's local date passes midnight", async () => {
    vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout", "Date"] });
    vi.setSystemTime(new Date(2026, 4, 1, 23, 59, 0));
    server.person = person({ profile: { birthDate: "2000-05-02" } });
    renderPage();
    expect(await screen.findByText("2000-05-02 (age 25)")).toBeTruthy();

    await act(() => vi.advanceTimersByTimeAsync(60_000));

    expect(screen.getByText("2000-05-02 (age 26)")).toBeTruthy();
  });

  it("asks for the videos in the pressed role", async () => {
    const user = userEvent.setup();
    renderPage();
    await screen.findByRole("link", { name: /Video 1/ });

    await user.click(screen.getByRole("radio", { name: "Director" }));

    await waitFor(() => {
      const last = sent("GET", "/api/videos").at(-1);
      expect(last?.url.searchParams.get("person")).toBe("7");
      expect(last?.url.searchParams.get("personRole")).toBe("director");
    });
  });

  // A profile leaves one Edit link; an empty profile offers a second one.
  const profiled = { profile: { birthplace: "Tokyo" } };

  it("opens the editor from Edit with the shown person, without reading it again", async () => {
    const user = userEvent.setup();
    server.person = person(profiled);
    renderPage();
    await user.click(await screen.findByRole("link", { name: "Edit" }));

    const name = await screen.findByRole<HTMLInputElement>("textbox", { name: "Name" });
    expect(name.value).toBe("Aoi");
    expect(sent("GET", "/api/people/7")).toHaveLength(1);
  });

  it("reads the person again on the next visit after Edit opened another tab", async () => {
    server.person = person(profiled);
    const first = renderPage();
    const edit = await screen.findByRole("link", { name: "Edit" });
    // jsdom cannot open a tab; stop its navigation after React has handled the click.
    const stop = (event: Event) => event.preventDefault();
    document.addEventListener("click", stop);
    try {
      fireEvent.click(edit, { ctrlKey: true });
    } finally {
      document.removeEventListener("click", stop);
    }
    first.unmount();
    server.person = person({ ...profiled, name: "Aoi Sora" });

    renderPage();
    expect(await screen.findByRole("heading", { name: "Aoi Sora" })).toBeTruthy();
  });

  it("titles a provisional person Unnamed person and offers Name… in place of Edit", async () => {
    const user = userEvent.setup();
    server.person = person({ ...profiled, name: "", provisional: true });
    renderPage();

    expect(await screen.findByRole("heading", { name: "Unnamed person" })).toBeTruthy();
    expect(document.title).toContain("Unnamed person");
    expect(screen.queryByRole("link", { name: "Edit" })).toBeNull();
    await user.click(screen.getByRole("link", { name: "Name…" }));

    const name = await screen.findByRole<HTMLInputElement>("textbox", { name: "Name" });
    expect(name.value).toBe("");
  });

  it("opens the video at a span's start, playing, from Faces in videos", async () => {
    const user = userEvent.setup();
    server.appearances = [
      {
        appearanceId: 3,
        videoId: 5,
        title: "Beach day",
        spans: [
          { start: 12.4, end: 100 },
          { start: 130, end: 150 },
        ],
        hasCrop: true,
      },
    ];
    renderPage();

    const crop = await screen.findByRole("img", { name: "Beach day" });
    expect(crop.getAttribute("src")).toBe("/api/face-appearances/3/crop");
    await user.click(screen.getByRole("link", { name: "Beach day from 2:10" }));

    expect((await screen.findByTestId("video-screen")).textContent).toBe(
      "/videos/5?t=130 autoplay",
    );
  });

  it("splits a face into a new person from Faces in videos and removes its row", async () => {
    const user = userEvent.setup();
    server.appearances = [
      { appearanceId: 3, videoId: 5, title: "Beach day", spans: [], hasCrop: true },
      { appearanceId: 4, videoId: 6, title: "Mountain", spans: [], hasCrop: true },
    ];
    renderPage();

    await user.click(
      await screen.findByRole("button", { name: "Not this person in Beach day" }),
    );

    expect(await screen.findByText("Split into a new unnamed person")).toBeTruthy();
    expect(sent("POST", "/api/people/7/appearances/3/split")).toHaveLength(1);
    expect(
      screen.queryByRole("button", { name: "Not this person in Beach day" }),
    ).toBeNull();
    expect(
      screen.getByRole("button", { name: "Not this person in Mountain" }),
    ).toBeTruthy();

    fireEvent.click(screen.getByRole("button", { name: "Open" }));
    await waitFor(() => expect(sent("GET", "/api/people/12").length).toBeGreaterThan(0));
  });

  it("reads the next page of Faces in videos when a split leaves no row shown", async () => {
    const user = userEvent.setup();
    server.appearances = [
      { appearanceId: 3, videoId: 5, title: "Beach day", spans: [], hasCrop: true },
    ];
    server.laterAppearances = [
      { appearanceId: 4, videoId: 6, title: "Mountain", spans: [], hasCrop: true },
    ];
    renderPage();

    await user.click(
      await screen.findByRole("button", { name: "Not this person in Beach day" }),
    );

    expect(
      await screen.findByRole("button", { name: "Not this person in Mountain" }),
    ).toBeTruthy();
    expect(
      sent("GET", "/api/people/7/appearances").filter(
        (request) => request.url.searchParams.get("cursor") === "page-2",
      ),
    ).toHaveLength(1);
  });

  function candidate(id: number, name: string): FaceCandidate {
    return { person: { id, name, provisional: name === "" }, similarity: 0.72 };
  }

  it("moves to the merged person's page when the shown person was merged away", async () => {
    const user = userEvent.setup();
    server.person = person({ name: "", provisional: true });
    server.candidates = [candidate(9, "Alice")];
    server.mergeTarget = person({ id: 9, name: "Alice" });
    renderPage();

    expect(await screen.findByText("72% similar")).toBeTruthy();
    await user.click(screen.getByRole("button", { name: "Same person as Alice" }));

    expect(await screen.findByRole("heading", { name: "Alice", level: 1 })).toBeTruthy();
    expect(screen.getByText('Merged into "Alice"')).toBeTruthy();
    expect(sent("POST", "/api/people/7/face-candidates/9/confirm")).toHaveLength(1);
  });

  it("keeps the page and reads the videos again when the shown person gains the other", async () => {
    const user = userEvent.setup();
    server.candidates = [candidate(9, "")];
    server.mergeTarget = person({ videoCount: 2 });
    renderPage();
    await screen.findByRole("link", { name: /Video 1/ });
    const videoReads = sent("GET", "/api/videos").length;

    await user.click(
      await screen.findByRole("button", { name: "Same person as Unnamed person" }),
    );

    expect(await screen.findByText('Merged "Unnamed person"')).toBeTruthy();
    expect(screen.getByRole("heading", { name: "Aoi", level: 1 })).toBeTruthy();
    expect(screen.queryByRole("heading", { name: "Same person?" })).toBeNull();
    await waitFor(() =>
      expect(sent("GET", "/api/videos").length).toBeGreaterThan(videoReads),
    );
  });

  it("removes a dismissed candidate without leaving the page", async () => {
    const user = userEvent.setup();
    server.candidates = [candidate(9, "Alice"), candidate(10, "Mio")];
    renderPage();

    await user.click(
      await screen.findByRole("button", { name: "Not the same as Alice" }),
    );

    await waitFor(() =>
      expect(screen.queryByRole("link", { name: "Open Alice" })).toBeNull(),
    );
    expect(screen.getByRole("link", { name: "Open Mio" })).toBeTruthy();
    expect(screen.getByRole("heading", { name: "Aoi", level: 1 })).toBeTruthy();
    expect(sent("POST", "/api/people/7/face-candidates/9/dismiss")).toHaveLength(1);
  });

  it("shows the gone state when the person no longer exists", async () => {
    server.person = null;
    renderPage();

    expect(await screen.findByText("This person no longer exists")).toBeTruthy();
    expect(
      screen.getByRole("link", { name: "Back to people" }).getAttribute("href"),
    ).toBe("/people");
  });
});
