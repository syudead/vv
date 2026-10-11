import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import AutoTagButton from "./AutoTagButton";

const toast = vi.fn();
vi.mock("../ui/Toast", () => ({ useToast: () => toast }));

const SETTINGS = "/api/settings/auto-tagging";

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function settings(enabled: boolean) {
  return {
    enabled,
    afterImport: false,
    threshold: 0.8,
    tentativeThreshold: 0.5,
    queue: { queued: 0, running: 0, done: 0, failed: 0 },
  };
}

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
    fetchMock.mockImplementation((input) =>
      Promise.resolve(
        String(input) === SETTINGS ? json(settings(true)) : json({ queued }, 202),
      ),
    );
    const user = userEvent.setup();
    render(<AutoTagButton videoId={7} />);
    await user.click(await screen.findByRole("button", { name: "Suggest tags" }));
    await waitFor(() => expect(toast).toHaveBeenCalledWith(message));
    const post = fetchMock.mock.calls.find(([input]) => String(input) !== SETTINGS);
    expect(String(post?.[0])).toBe("/api/videos/7/auto-tag");
    expect(post?.[1]?.method).toBe("POST");
  });

  it("shows nothing while auto-tagging is off", async () => {
    fetchMock.mockResolvedValue(json(settings(false)));
    const { container } = render(<AutoTagButton videoId={7} />);
    await waitFor(() => expect(fetchMock).toHaveBeenCalled());
    await Promise.resolve();
    expect(container.innerHTML).toBe("");
    expect(screen.queryByRole("button", { name: "Suggest tags" })).toBeNull();
  });
});
