import { fireEvent, render, screen, within } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import type { UiText } from "@/i18n";

import { ImageUploadDialog, pickFile } from "./ImageUploadDialog";

function png(name = "shot.png") {
  return new File(["png"], name, { type: "image/png" });
}

function setup(onFile = vi.fn<(file: File) => UiText | null>(() => null)) {
  render(
    <ImageUploadDialog
      open
      onOpenChange={vi.fn()}
      title="Upload a face"
      accept="image/png"
      onFile={onFile}
    />,
  );
  return { dialog: screen.getByRole("dialog", { name: "Upload a face" }), onFile };
}

describe("ImageUploadDialog", () => {
  it("passes a chosen, dropped or pasted file to onFile", () => {
    const { dialog, onFile } = setup();
    const chosen = png("chosen.png");
    const dropped = png("dropped.png");
    const pasted = png("pasted.png");

    fireEvent.change(within(dialog).getByTestId("image-upload-input"), {
      target: { files: [chosen] },
    });
    const drop = fireEvent.drop(dialog, {
      dataTransfer: { types: ["Files"], files: [dropped] },
    });
    fireEvent.paste(dialog, { clipboardData: { files: [pasted] } });

    expect(onFile.mock.calls.map(([file]) => file)).toEqual([chosen, dropped, pasted]);
    // A cancelled drop keeps the browser from leaving the app to open the file.
    expect(drop).toBe(false);
  });

  it("shows the reason onFile refuses a file with", () => {
    const { dialog } = setup(vi.fn(() => "Choose a PNG." as UiText));
    fireEvent.paste(dialog, { clipboardData: { files: [png()] } });
    expect(within(dialog).getByRole("alert").textContent).toBe("Choose a PNG.");
  });

  it("opens the file picker from the drop zone", () => {
    const { dialog } = setup();
    const input = within(dialog).getByTestId<HTMLInputElement>("image-upload-input");
    const click = vi.spyOn(input, "click");
    fireEvent.click(
      within(dialog).getByRole("button", { name: /Drop an image here, or click/ }),
    );
    expect(click).toHaveBeenCalledTimes(1);
  });
});

describe("pickFile", () => {
  it("prefers an image and otherwise returns the first file for the caller to refuse", () => {
    const text = new File(["a"], "a.txt", { type: "text/plain" });
    const image = png();
    expect(pickFile([text, image])).toBe(image);
    expect(pickFile([text])).toBe(text);
    expect(pickFile([])).toBeNull();
  });
});
