import { fireEvent, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { installPersonServer, renderPage, sent } from "../../testing/personPage";
import { encodeFaceFrame } from "./faceFrameImage";

vi.mock("./faceFrameImage", () => ({
  encodeFaceFrame: vi.fn(() =>
    Promise.resolve(new Blob(["face"], { type: "image/jpeg" })),
  ),
}));

installPersonServer();

/** jsdom decodes no images, so every picture loads as 1000 by 500 px. */
class LoadedImage {
  naturalWidth = 1000;
  naturalHeight = 500;
  onload: (() => void) | null = null;
  onerror: (() => void) | null = null;
  set src(_: string) {
    queueMicrotask(() => this.onload?.());
  }
}

beforeEach(() => {
  vi.stubGlobal("Image", LoadedImage);
  vi.stubGlobal(
    "URL",
    class extends URL {
      static createObjectURL = () => "blob:face";
      static revokeObjectURL = () => undefined;
    },
  );
});

afterEach(() => {
  vi.unstubAllGlobals();
  vi.mocked(encodeFaceFrame).mockClear();
});

function png() {
  return new File(["face"], "face.png", { type: "image/png" });
}

async function framing() {
  const dialog = await screen.findByRole("dialog", { name: /Frame the face/ });
  await within(dialog).findByRole("application", { name: "Face area" });
  return dialog;
}

describe("FaceCropDialog", () => {
  it("frames a chosen image before saving the cut as the face", async () => {
    const user = userEvent.setup();
    renderPage();
    await user.click(await screen.findByRole("button", { name: "Add a face" }));
    const upload = await screen.findByRole("dialog", { name: "Upload a face" });

    await user.upload(within(upload).getByTestId("image-upload-input"), png());
    const dialog = await framing();
    expect(sent("PUT", "/api/people/7/image")).toHaveLength(0);
    await user.click(within(dialog).getByRole("button", { name: "Set as face" }));

    // The widest 2:3 portrait centred in a 1000 by 500 image.
    const [, rect] = vi.mocked(encodeFaceFrame).mock.calls[0] ?? [];
    expect(rect?.width).toBeCloseTo(1000 / 3);
    expect(rect?.height).toBeCloseTo(500);
    expect(rect?.x).toBeCloseTo(1000 / 3);
    expect(rect?.y).toBeCloseTo(0);
    const image = await screen.findByRole("img", { name: "Aoi" });
    expect(image.getAttribute("src")).toBe("/api/people/7/image?v=3");
    expect(sent("PUT", "/api/people/7/image")).toHaveLength(1);
    expect(screen.queryByRole("dialog", { name: /Frame the face/ })).toBeNull();
  });

  it("opens an image pasted in the upload dialog for framing and sends nothing when cancelled", async () => {
    const user = userEvent.setup();
    renderPage();
    await user.click(await screen.findByRole("button", { name: "Add a face" }));

    fireEvent.paste(await screen.findByRole("dialog", { name: "Upload a face" }), {
      clipboardData: { files: [png()] },
    });
    const dialog = await framing();
    await user.click(within(dialog).getByRole("button", { name: "Cancel" }));

    await waitFor(() =>
      expect(screen.queryByRole("dialog", { name: /Frame the face/ })).toBeNull(),
    );
    expect(sent("PUT", "/api/people/7/image")).toHaveLength(0);
  });
});
