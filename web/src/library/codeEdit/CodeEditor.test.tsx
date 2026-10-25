import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import { RequestFailed } from "../../api/client";
import { TooltipProvider } from "../../ui/shadcn/tooltip";
import CodeEditor from "./CodeEditor";
import type { CodeEditItem, CodeEditRow } from "./rows";

afterEach(cleanup);

function row(id: number, fields: { fileTitle?: string; code?: string }): CodeEditRow {
  return {
    video: {
      id,
      virtual: fields.fileTitle === undefined,
      title: `Video ${String(id)}`,
      fileTitle: fields.fileTitle,
      contentVersion: `v${String(id)}`,
      sizeBytes: 0,
      addedAt: "2026-10-01T00:00:00Z",
      updatedAt: "2026-10-01T00:00:00Z",
      fileCreatedAt: "2026-10-01T00:00:00Z",
      fileModifiedAt: "2026-10-01T00:00:00Z",
      playable: true,
      probeState: "done",
      thumbnailState: "done",
      previewState: "done",
      tags: [],
      public: false,
    },
    code: fields.code,
  };
}

function renderEditor(
  rows: CodeEditRow[],
  onApply = vi.fn<(items: CodeEditItem[]) => Promise<void>>(() => Promise.resolve()),
) {
  const onClose = vi.fn<() => void>();
  render(
    <TooltipProvider>
      <CodeEditor rows={rows} onApply={onApply} onClose={onClose} />
    </TooltipProvider>,
  );
  return { onApply, onClose, user: userEvent.setup() };
}

function codeInput(id: number): HTMLInputElement {
  return screen.getByRole("textbox", { name: `Code of Video ${String(id)}` });
}

/** The wide State column; the narrow copy of the marks in the code cell is hidden by CSS only. */
function stateCell(id: number): HTMLElement {
  const cells = codeInput(id).closest("tr")?.querySelectorAll("td");
  const cell = cells?.[cells.length - 1];
  if (!(cell instanceof HTMLElement)) throw new Error("row without cells");
  return cell;
}

function applyButton(): HTMLElement {
  return screen.getByRole("button", { name: /^Apply to/ });
}

describe("CodeEditor", () => {
  it("shows existing codes as they are and suggests file names on rows without one", () => {
    renderEditor([
      row(1, { fileTitle: "old name", code: "XYZ-1" }),
      row(2, { fileTitle: "[xxx] ABC-123 1080p" }),
    ]);

    expect(codeInput(1).value).toBe("XYZ-1");
    expect(within(stateCell(1)).queryByText("Suggested")).toBeNull();
    expect(codeInput(2).value).toBe("[xxx] ABC-123 1080p");
    expect(codeInput(2).classList.contains("text-muted-foreground")).toBe(true);
    expect(within(stateCell(2)).getByText("Suggested")).toBeTruthy();
    expect(applyButton().textContent).toBe("Apply to 1 video");
  });

  it("applies the typed pattern to suggested rows only, keeping non-matching names", async () => {
    const { user } = renderEditor([
      row(1, { fileTitle: "[xxx] ABC-123 1080p" }),
      row(2, { fileTitle: "holiday" }),
      row(3, { fileTitle: "DEF-9 copy", code: "KEEP-1" }),
    ]);

    // user-event reads "[" as a key name, so "[[" types one bracket.
    await user.type(screen.getByLabelText("Pattern"), String.raw`[[A-Z]+-\d+`);

    expect(codeInput(1).value).toBe("ABC-123");
    expect(codeInput(2).value).toBe("holiday");
    expect(codeInput(3).value).toBe("KEEP-1");
  });

  it("shows an invalid pattern's error and keeps the last valid values", async () => {
    const { user } = renderEditor([row(1, { fileTitle: "ABC-123 x" })]);
    const pattern = screen.getByLabelText("Pattern");

    await user.type(pattern, "[[A-Z]+");
    expect(codeInput(1).value).toBe("ABC");
    await user.type(pattern, "(");

    expect(pattern.getAttribute("aria-invalid")).toBe("true");
    expect(screen.getByText(/^Not a valid pattern: /)).toBeTruthy();
    expect(codeInput(1).value).toBe("ABC");
  });

  it("warns on both rows with one value and still applies them", async () => {
    const { user, onApply } = renderEditor([
      row(1, { fileTitle: "ABC-1" }),
      row(2, { fileTitle: "ABC-1" }),
    ]);

    expect(within(stateCell(1)).getByText("Duplicate")).toBeTruthy();
    expect(within(stateCell(2)).getByText("Duplicate")).toBeTruthy();
    expect(screen.getByText("2 rows share a code")).toBeTruthy();
    expect(applyButton().hasAttribute("aria-disabled")).toBe(false);

    await user.click(applyButton());
    expect(onApply).toHaveBeenCalledWith([
      { videoId: 1, contentVersion: "v1", code: "ABC-1" },
      { videoId: 2, contentVersion: "v2", code: "ABC-1" },
    ]);
  });

  it("shows a typed value over 100 code points as the cut value Apply sends", async () => {
    const { user, onApply } = renderEditor([
      row(1, { fileTitle: "x", code: "A".repeat(99) }),
    ]);
    const cut = `${"A".repeat(99)}B`;

    await user.type(codeInput(1), "BC");

    expect(codeInput(1).value).toBe(cut);
    expect(within(stateCell(1)).getByText("Cut to 100")).toBeTruthy();
    await user.click(applyButton());
    expect(onApply).toHaveBeenCalledWith([
      { videoId: 1, contentVersion: "v1", code: cut },
    ]);
  });

  it("disables Apply to 0 videos when only virtual videos are selected", async () => {
    const { user, onApply } = renderEditor([row(1, {}), row(2, {})]);

    expect(codeInput(1).value).toBe("");
    expect(applyButton().textContent).toBe("Apply to 0 videos");
    expect(applyButton().getAttribute("aria-disabled")).toBe("true");
    expect(screen.getByText("Apply writes 0 of 2 videos")).toBeTruthy();

    await user.click(applyButton());
    expect(onApply).not.toHaveBeenCalled();
  });

  it("leaves an emptied row out of the count and the items", async () => {
    const { user, onApply } = renderEditor([
      row(1, { fileTitle: "ABC-1" }),
      row(2, { fileTitle: "DEF-2" }),
    ]);

    await user.clear(codeInput(1));

    expect(within(stateCell(1)).queryByText("Suggested")).toBeNull();
    expect(within(stateCell(1)).queryByText("Changed")).toBeNull();
    expect(screen.getByText("Apply writes 1 of 2 videos")).toBeTruthy();
    await user.click(applyButton());
    expect(onApply).toHaveBeenCalledWith([
      { videoId: 2, contentVersion: "v2", code: "DEF-2" },
    ]);
  });

  it("keeps every value and marks the named row when Apply fails", async () => {
    const onApply = vi.fn<(items: CodeEditItem[]) => Promise<void>>(() =>
      Promise.reject(
        new RequestFailed(409, "conflict", "changed", {
          reason: "content_changed",
          field: "items[1].contentVersion",
        }),
      ),
    );
    const { user, onClose } = renderEditor(
      [row(1, { fileTitle: "ABC-1" }), row(2, { fileTitle: "DEF-2" })],
      onApply,
    );
    await user.type(codeInput(1), "x");

    await user.click(applyButton());

    expect(await screen.findByText(/^Couldn't apply the codes: /)).toBeTruthy();
    expect(codeInput(2).getAttribute("aria-invalid")).toBe("true");
    expect(document.activeElement).toBe(codeInput(2));
    expect(codeInput(1).value).toBe("ABC-1x");
    expect(onClose).not.toHaveBeenCalled();
  });

  it("asks before discarding a change on Cancel", async () => {
    const { user, onClose } = renderEditor([row(1, { fileTitle: "ABC-1" })]);
    await user.type(codeInput(1), "x");

    await user.click(screen.getByRole("button", { name: "Cancel" }));

    expect(
      screen.getByRole("alertdialog", { name: "Discard these changes?" }),
    ).toBeTruthy();
    expect(onClose).not.toHaveBeenCalled();
  });
});
