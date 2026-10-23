import { act, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { pickFile, useImageIntake } from "./ImageDrop";

function png(name = "shot.png") {
  return new File(["png"], name, { type: "image/png" });
}

/** jsdom has no DataTransfer, so carry only the fields the hook reads. */
function fire(
  type: string,
  data: { types: string[]; files: File[] },
  target: EventTarget = document.body,
) {
  const event = new Event(type, { bubbles: true, cancelable: true });
  const key = type === "paste" ? "clipboardData" : "dataTransfer";
  Object.defineProperty(event, key, { value: { ...data, dropEffect: "none" } });
  act(() => {
    target.dispatchEvent(event);
  });
  return event;
}

function setup(scope: "page" | "dialog" = "page", enabled = true) {
  const onFile = vi.fn();
  const hook = renderHook(() => useImageIntake({ enabled, onFile, scope }));
  return { ...hook, onFile };
}

afterEach(() => {
  document.body.innerHTML = "";
});

describe("useImageIntake", () => {
  it("takes a pasted image and keeps the browser from pasting it", () => {
    const { onFile } = setup();
    const file = png();
    const event = fire("paste", { types: ["Files"], files: [file] });
    expect(onFile).toHaveBeenCalledWith(file);
    expect(event.defaultPrevented).toBe(true);
  });

  it("leaves text pasted into a field to the field", () => {
    const { onFile } = setup();
    const field = document.body.appendChild(document.createElement("input"));
    const event = fire(
      "paste",
      { types: ["text/plain", "Files"], files: [png()] },
      field,
    );
    expect(onFile).not.toHaveBeenCalled();
    expect(event.defaultPrevented).toBe(false);
  });

  it("shows the drag over the window and takes the dropped file", () => {
    const { result, onFile } = setup();
    const file = png();
    const data = { types: ["Files"], files: [file] };
    fire("dragenter", data);
    expect(result.current.dragging).toBe(true);
    const drop = fire("drop", data);
    expect(drop.defaultPrevented).toBe(true);
    expect(onFile).toHaveBeenCalledWith(file);
    expect(result.current.dragging).toBe(false);
  });

  it("ends the drag when it leaves the window", () => {
    const { result } = setup();
    const data = { types: ["Files"], files: [] };
    const inner = document.body.appendChild(document.createElement("div"));
    fire("dragenter", data);
    fire("dragenter", data, inner);
    fire("dragleave", data);
    expect(result.current.dragging).toBe(true);
    fire("dragleave", data, inner);
    expect(result.current.dragging).toBe(false);
  });

  it("steps aside on the page while a dialog is open, but not in the dialog", () => {
    const page = setup("page");
    const dialog = setup("dialog");
    document.body.appendChild(document.createElement("div")).role = "dialog";
    const file = png();
    fire("paste", { types: ["Files"], files: [file] });
    expect(page.onFile).not.toHaveBeenCalled();
    expect(dialog.onFile).toHaveBeenCalledWith(file);
  });

  it("takes nothing while disabled", () => {
    const { result, onFile } = setup("page", false);
    fire("dragenter", { types: ["Files"], files: [png()] });
    fire("paste", { types: ["Files"], files: [png()] });
    expect(result.current.dragging).toBe(false);
    expect(onFile).not.toHaveBeenCalled();
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
