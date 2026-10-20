import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { ToastProvider } from "../ui/Toast";
import { TooltipProvider } from "../ui/shadcn/tooltip";
import EditCreditsDialog from "./EditCreditsDialog";

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function credit(id: number, name: string) {
  return {
    id,
    name,
    aliases: [],
    externalIds: [],
    videoCount: 2,
    createdAt: "2026-09-01T00:00:00Z",
  };
}

const server = {
  edit: vi.fn<(body: unknown) => Response>(),
  createPerson: vi.fn<(body: unknown) => Response>(),
};

beforeEach(() => {
  // jsdom lacks the pointer capture API that Radix Select calls.
  Element.prototype.hasPointerCapture ??= () => false;
  server.edit.mockReset().mockReturnValue(json({ applied: 3 }));
  server.createPerson.mockReset().mockReturnValue(json(credit(50, "Eve")));
  vi.stubGlobal(
    "fetch",
    vi.fn<typeof fetch>((input, init) => {
      const url = new URL(String(input), "http://localhost");
      const method = init?.method ?? "GET";
      const body: unknown =
        init?.body === undefined ? undefined : JSON.parse(String(init.body));
      if (url.pathname === "/api/people" && method === "GET") {
        return Promise.resolve(json({ items: [credit(7, "Alice")] }));
      }
      if (url.pathname === "/api/people" && method === "POST") {
        return Promise.resolve(server.createPerson(body));
      }
      if (url.pathname === "/api/video-credits" && method === "POST") {
        return Promise.resolve(server.edit(body));
      }
      return Promise.resolve(json({ code: "not_found", message: "not found" }, 404));
    }),
  );
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function renderDialog() {
  const onClose = vi.fn<() => void>();
  render(
    <ToastProvider>
      <TooltipProvider>
        <EditCreditsDialog
          count={3}
          selectedIds={[1, 2]}
          smartFolderIds={[9]}
          onClose={onClose}
        />
      </TooltipProvider>
    </ToastProvider>,
  );
  return onClose;
}

describe("EditCreditsDialog", () => {
  it("sends only the touched roles in one request, then reports the applied count and closes", async () => {
    const user = userEvent.setup();
    const onClose = renderDialog();
    expect(
      screen.getByRole<HTMLButtonElement>("button", { name: "Apply" }).disabled,
    ).toBe(true);

    await user.type(screen.getByRole("combobox", { name: "Add a performer" }), "Ali");
    await user.click(await screen.findByRole("option", { name: /^Alice/ }));
    await user.click(screen.getByRole("combobox", { name: "How to change Makers" }));
    await user.click(await screen.findByRole("option", { name: "Replace" }));
    expect(screen.getByText("Clears the makers on every selected video")).toBeTruthy();
    await user.click(screen.getByRole("button", { name: "Apply" }));

    await waitFor(() => expect(onClose).toHaveBeenCalled());
    expect(server.edit).toHaveBeenCalledTimes(1);
    expect(server.edit).toHaveBeenCalledWith({
      videoIds: [1, 2],
      smartFolderIds: [9],
      edits: [
        { kind: "person", role: "performer", action: "add", ids: [7] },
        { kind: "organization", role: "maker", action: "replace", ids: [] },
      ],
    });
    expect(await screen.findByText("Updated the credits of 3 videos")).toBeTruthy();
  });

  it("creates a name typed in two roles once and sends its ID for both", async () => {
    const user = userEvent.setup();
    renderDialog();

    await user.type(screen.getByRole("combobox", { name: "Add a performer" }), "Eve");
    await user.click(await screen.findByRole("option", { name: 'Create "Eve"' }));
    await user.type(screen.getByRole("combobox", { name: "Add a director" }), "Eve");
    await user.click(await screen.findByRole("option", { name: "Eve, new" }));
    await user.click(screen.getByRole("button", { name: "Apply" }));

    await waitFor(() => expect(server.edit).toHaveBeenCalled());
    expect(server.createPerson).toHaveBeenCalledTimes(1);
    expect(server.createPerson).toHaveBeenCalledWith({ name: "Eve" });
    expect(server.edit.mock.calls[0]?.[0]).toMatchObject({
      edits: [
        { kind: "person", role: "performer", action: "add", ids: [50] },
        { kind: "person", role: "director", action: "add", ids: [50] },
      ],
    });
  });

  it("keeps the dialog and the choices open with the reason when the request fails", async () => {
    server.edit.mockReturnValue(
      json(
        {
          code: "invalid_request",
          reason: "work_info_too_many",
          message: "edits[0].ids must have at most 100 entries.",
        },
        400,
      ),
    );
    const user = userEvent.setup();
    const onClose = renderDialog();

    await user.type(screen.getByRole("combobox", { name: "Add a performer" }), "Ali");
    await user.click(await screen.findByRole("option", { name: /^Alice/ }));
    await user.click(screen.getByRole("button", { name: "Apply" }));

    expect(await screen.findByText(/^Couldn't edit the credits:/)).toBeTruthy();
    expect(onClose).not.toHaveBeenCalled();
    expect(screen.getByText("Alice")).toBeTruthy();
    expect(
      screen.getByRole<HTMLButtonElement>("button", { name: "Apply" }).disabled,
    ).toBe(false);
  });
});
