import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import AutoTagButton from "./AutoTagButton";

const toast = vi.fn();
vi.mock("../ui/Toast", () => ({ useToast: () => toast }));

describe("AutoTagButton", () => {
  const fetchMock = vi.fn<typeof fetch>();

  beforeEach(() => {
    toast.mockReset();
    fetchMock.mockReset();
    vi.stubGlobal("fetch", fetchMock);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it.each([
    [
      true,
      "Asking the classifier for tags. Matching tags are added when it answers, which can take a few minutes.",
    ],
    [false, "This video is already being tagged."],
  ])("queues the video (queued=%s) and says so", async (queued, message) => {
    fetchMock.mockResolvedValue(
      new Response(JSON.stringify({ queued }), {
        status: 202,
        headers: { "Content-Type": "application/json" },
      }),
    );
    const user = userEvent.setup();
    render(<AutoTagButton videoId={7} />);
    await user.click(screen.getByRole("button", { name: "Suggest tags" }));
    await waitFor(() => expect(toast).toHaveBeenCalledWith(message));
    expect(String(fetchMock.mock.calls[0]?.[0])).toBe("/api/videos/7/auto-tag");
    expect(fetchMock.mock.calls[0]?.[1]?.method).toBe("POST");
  });
});
