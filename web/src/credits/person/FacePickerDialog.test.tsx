import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import {
  installPersonServer,
  person,
  renderPage,
  sent,
  server,
} from "../../testing/personPage";
import { encodeFaceFrame } from "./faceFrameImage";

vi.mock("./faceFrameImage", () => ({
  encodeFaceFrame: vi.fn(() =>
    Promise.resolve(new Blob(["face"], { type: "image/jpeg" })),
  ),
}));

installPersonServer();

/** Frames whose load waits for release; empty lets every frame load at once. */
const held: (() => void)[] = [];
let holding = false;

/** jsdom decodes no images, so every frame loads as 1000 by 500 px, at once unless held. */
class LoadedImage {
  naturalWidth = 1000;
  naturalHeight = 500;
  onload: (() => void) | null = null;
  onerror: (() => void) | null = null;
  set src(_: string) {
    const load = () => this.onload?.();
    if (holding) held.push(load);
    else queueMicrotask(load);
  }
}

beforeEach(() => {
  vi.stubGlobal("Image", LoadedImage);
});

afterEach(() => {
  vi.unstubAllGlobals();
  holding = false;
  held.length = 0;
  vi.mocked(encodeFaceFrame).mockClear();
});

const beach = {
  appearanceId: 3,
  videoId: 5,
  title: "Beach day",
  spans: [{ start: 10, end: 15 }],
  hasCrop: true,
  focus: { time: 12.5, x: 0.5, y: 0.2, w: 0.05 },
};

async function openPicker(user: ReturnType<typeof userEvent.setup>) {
  await user.click(await screen.findByRole("button", { name: "Change face" }));
  await user.click(await screen.findByRole("menuitem", { name: "Choose from videos…" }));
  return screen.findByRole("dialog", { name: /Choose a face/ });
}

describe("FacePickerDialog", () => {
  it("saves nothing while the chosen moment's frame is still loading", async () => {
    const user = userEvent.setup();
    server.person = person({ imageRevision: 2 });
    server.appearances = [beach];
    renderPage();
    const dialog = await openPicker(user);
    await within(dialog).findByText("At 0:12");
    await waitFor(() =>
      expect(dialog.querySelector("img[src*='/frame']")).not.toBeNull(),
    );

    holding = true;
    await user.click(within(dialog).getByRole("button", { name: "1 second later" }));
    const save = within(dialog).getByRole("button", { name: "Set as face" });
    expect(save.getAttribute("aria-disabled")).toBe("true");
    await user.click(save);

    expect(encodeFaceFrame).not.toHaveBeenCalled();
    expect(sent("PUT", "/api/people/7/image")).toHaveLength(0);
  });

  it("keeps the dialog open with the refusal when the face is not accepted", async () => {
    const user = userEvent.setup();
    server.person = person({ imageRevision: 2 });
    server.appearances = [beach];
    server.imageRefusal = "person_image_invalid";
    renderPage();
    const dialog = await openPicker(user);
    await waitFor(() =>
      expect(dialog.querySelector("img[src*='/frame']")).not.toBeNull(),
    );

    await user.click(within(dialog).getByRole("button", { name: "Set as face" }));

    expect(
      await within(dialog).findByText(
        "This image can't be used. Choose a PNG, JPEG or still WebP.",
      ),
    ).toBeTruthy();
    expect(screen.getByRole("dialog", { name: /Choose a face/ })).toBeTruthy();
  });

  it("moves the frame with the arrow keys before cutting it", async () => {
    const user = userEvent.setup();
    server.person = person({ imageRevision: 2 });
    server.appearances = [beach];
    renderPage();
    const dialog = await openPicker(user);

    // One step is a hundredth of the frame's width: 10 of its 1000 px.
    const area = await within(dialog).findByRole("application", { name: "Face area" });
    area.focus();
    await user.keyboard("{ArrowRight}");
    await user.click(within(dialog).getByRole("button", { name: "Set as face" }));

    await waitFor(() => expect(encodeFaceFrame).toHaveBeenCalled());
    const [, rect] = vi.mocked(encodeFaceFrame).mock.calls[0] ?? [];
    expect((rect?.x ?? 0) + (rect?.width ?? 0) / 2).toBeCloseTo(510);
  });

  it("says so when the person has no faces from videos", async () => {
    const user = userEvent.setup();
    server.person = person({ imageRevision: 2 });
    server.appearances = [];
    renderPage();
    const dialog = await openPicker(user);

    expect(await within(dialog).findByText(/No faces from videos yet/)).toBeTruthy();
    expect(
      within(dialog)
        .getByRole("button", { name: "Set as face" })
        .getAttribute("aria-disabled"),
    ).toBe("true");
  });

  it("reads further pages until it finds faces that can be framed", async () => {
    const user = userEvent.setup();
    server.person = person({ imageRevision: 2 });
    server.appearances = [
      { appearanceId: 4, title: "Gone video", spans: [], hasCrop: true },
    ];
    server.laterAppearances = [beach];
    renderPage();
    const dialog = await openPicker(user);

    expect(
      await within(dialog).findByRole("radio", { name: "Face in Beach day" }),
    ).toBeTruthy();
  });

  it("saves a portrait cut from the chosen face's frame, a second later", async () => {
    const user = userEvent.setup();
    server.person = person({ imageRevision: 2 });
    server.appearances = [
      {
        appearanceId: 3,
        videoId: 5,
        title: "Beach day",
        spans: [{ start: 10, end: 15 }],
        hasCrop: true,
        focus: { time: 12.5, x: 0.5, y: 0.2, w: 0.05 },
      },
      { appearanceId: 4, title: "Gone video", spans: [], hasCrop: true },
    ];
    renderPage();

    await user.click(await screen.findByRole("button", { name: "Change face" }));
    await user.click(
      await screen.findByRole("menuitem", { name: "Choose from videos…" }),
    );
    const dialog = await screen.findByRole("dialog", { name: /Choose a face/ });

    const [list] = sent("GET", "/api/people/7/appearances");
    expect(list?.url.searchParams.get("sort")).toBe("mostSimilar");
    // A face whose video is gone has no frame to cut, so only one face is offered.
    expect(within(dialog).getAllByRole("radio")).toHaveLength(1);
    expect(within(dialog).getByRole("radio", { name: "Face in Beach day" })).toBeTruthy();
    expect(within(dialog).getByText("At 0:12")).toBeTruthy();

    await user.click(within(dialog).getByRole("button", { name: "1 second later" }));
    expect(within(dialog).getByText("At 0:13")).toBeTruthy();
    await waitFor(() =>
      expect(dialog.querySelector("img[src*='/frame']")?.getAttribute("src")).toBe(
        "/api/videos/5/frame?positionMs=13500",
      ),
    );
    await user.click(within(dialog).getByRole("button", { name: "Set as face" }));

    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    const [image, rect] = vi.mocked(encodeFaceFrame).mock.calls[0] ?? [];
    expect(image).toBeInstanceOf(LoadedImage);
    expect(rect?.width).toBeCloseTo(110);
    expect((rect?.x ?? 0) + (rect?.width ?? 0) / 2).toBeCloseTo(500);
    const [upload] = sent("PUT", "/api/people/7/image");
    expect((upload?.body as FormData).get("image")).toBeInstanceOf(Blob);
    expect(screen.getByRole("img", { name: "Aoi" }).getAttribute("src")).toBe(
      "/api/people/7/image?v=3",
    );
  });
});
