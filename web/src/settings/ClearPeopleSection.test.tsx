import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import ClearPeopleSection from "./ClearPeopleSection";

const toast = vi.fn();
vi.mock("../ui/Toast", () => ({ useToast: () => toast }));

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

describe("ClearPeopleSection", () => {
  const fetchMock = vi.fn<typeof fetch>();

  beforeEach(() => {
    toast.mockReset();
    fetchMock.mockReset();
    vi.stubGlobal("fetch", fetchMock);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  async function confirmDelete() {
    render(<ClearPeopleSection />);
    await userEvent.click(screen.getByRole("button", { name: "Delete all people…" }));
    const dialog = await screen.findByRole("alertdialog");
    expect(fetchMock).not.toHaveBeenCalled();
    await userEvent.click(
      within(dialog).getByRole("button", { name: "Delete all people" }),
    );
    return dialog;
  }

  it("deletes every person only after the confirmation", async () => {
    fetchMock.mockResolvedValue(json({ deleted: 2, queued: 3 }));
    await confirmDelete();

    await waitFor(() =>
      expect(toast).toHaveBeenCalledWith(
        "2 people were deleted; 3 videos were queued for face analysis",
      ),
    );
    const [url, init] = fetchMock.mock.calls[0] ?? [];
    expect([String(url), init?.method]).toEqual(["/api/people", "DELETE"]);
    expect(screen.queryByRole("alertdialog")).toBeNull();
  });

  it("keeps the dialog open with the reason when deleting fails", async () => {
    fetchMock.mockResolvedValue(json({ code: "internal", message: "disk full" }, 500));
    const dialog = await confirmDelete();

    expect(await within(dialog).findByText(/Couldn't delete the people/)).toBeDefined();
    expect(toast).not.toHaveBeenCalled();
  });
});
