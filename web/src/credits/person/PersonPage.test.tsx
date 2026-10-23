import { act, fireEvent, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import {
  person,
  renderPage,
  sent,
  server,
  installPersonServer,
} from "../../testing/personPage";

installPersonServer();

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

  it("uploads an image pasted on the page as the face", async () => {
    renderPage();
    await screen.findByRole("button", { name: "Add a face" });

    const paste = new Event("paste", { bubbles: true, cancelable: true });
    const file = new File(["face"], "face.png", { type: "image/png" });
    Object.defineProperty(paste, "clipboardData", {
      value: { types: ["Files"], files: [file] },
    });
    act(() => {
      document.body.dispatchEvent(paste);
    });

    await screen.findByRole("img", { name: "Aoi" });
    const [upload] = sent("PUT", "/api/people/7/image");
    expect(((upload?.body as FormData).get("image") as File).name).toBe("face.png");
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

  it("shows a shelf with every video for each role the person has videos in", async () => {
    server.videoCount = 11;
    renderPage();
    await screen.findByRole("heading", { name: "Performer", level: 2 });

    const roles = sent("GET", "/api/videos").map((request) =>
      request.url.searchParams.get("personRole"),
    );
    expect(roles).toEqual(
      expect.arrayContaining([null, "performer", "director", "photographer", "streamer"]),
    );
    expect(screen.queryByRole("heading", { name: "Director", level: 2 })).toBeNull();
    expect(await screen.findByRole("link", { name: /Video 11/ })).toBeTruthy();
    expect(screen.queryByRole("button", { name: /See all/ })).toBeNull();
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
    // Let the shelves mount before the test's server goes away.
    await screen.findByRole("link", { name: /Video 1/ });
  });

  it("titles a provisional person by their number and offers Name… in place of Edit", async () => {
    const user = userEvent.setup();
    server.person = person({ ...profiled, name: "", provisional: true });
    renderPage();

    expect(await screen.findByRole("heading", { name: "#7" })).toBeTruthy();
    expect(document.title).toContain("#7");
    expect(screen.queryByRole("link", { name: "Edit" })).toBeNull();
    await user.click(screen.getByRole("link", { name: "Name…" }));

    const name = await screen.findByRole<HTMLInputElement>("textbox", { name: "Name" });
    expect(name.value).toBe("");
  });

  it("leads to the faces page from Faces, counting the pairs to review", async () => {
    const user = userEvent.setup();
    server.faceCandidates = [
      { person: { id: 9, name: "Alice", provisional: false }, similarity: 0.72 },
    ];
    renderPage();

    await user.click(await screen.findByRole("link", { name: "Faces, 1 to review" }));

    expect(await screen.findByRole("heading", { name: "Faces", level: 1 })).toBeTruthy();
    expect(await screen.findByRole("group", { name: "Pair with Alice" })).toBeTruthy();
  });

  it("hides Faces while face recognition is off", async () => {
    server.faceRecognition = false;
    server.faceCandidates = [
      { person: { id: 9, name: "Alice", provisional: false }, similarity: 0.72 },
    ];
    renderPage();
    await screen.findByRole("link", { name: /Video 1/ });

    await waitFor(() =>
      expect(sent("GET", "/api/settings/face-recognition")).toHaveLength(1),
    );
    expect(screen.queryByRole("link", { name: /^Faces/ })).toBeNull();
    expect(sent("GET", "/api/people/7/face-candidates")).toHaveLength(0);
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
