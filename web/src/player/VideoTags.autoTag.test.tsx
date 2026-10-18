import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { VideoTag } from "../api/client";
import { __resetTagsForTest } from "../api/tags";
import { ToastProvider } from "../ui/Toast";
import VideoTags from "./VideoTags";

function json(body: unknown, status = 200): Response {
  return new Response(body === null ? null : JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

const suggested: VideoTag = {
  kind: null,
  id: 5,
  name: "cat",
  manual: true,
  fromFolder: false,
  tentative: false,
  unconfirmed: true,
};
const kept: VideoTag = {
  kind: null,
  id: 6,
  name: "dog",
  manual: true,
  fromFolder: false,
  tentative: false,
};

describe("VideoTags suggested tags", () => {
  const fetchMock = vi.fn<typeof fetch>();
  let settleStatus = 204;

  beforeEach(() => {
    __resetTagsForTest();
    settleStatus = 204;
    fetchMock.mockReset();
    fetchMock.mockImplementation((input, init) => {
      const url = String(input);
      if (url === "/api/tags") return Promise.resolve(json({ items: [] }));
      if (url.startsWith("/api/videos/7/tags/") && init?.method === "POST")
        return Promise.resolve(
          settleStatus === 204
            ? new Response(null, { status: 204 })
            : json({ code: "not_found", message: "gone" }, settleStatus),
        );
      return Promise.reject(new Error(`unexpected request: ${url}`));
    });
    vi.stubGlobal("fetch", fetchMock);
  });

  afterEach(() => vi.unstubAllGlobals());

  function renderTags(onChanged = vi.fn(), onStaleVideo = vi.fn()) {
    render(
      <MemoryRouter>
        <ToastProvider>
          <VideoTags
            videoId={7}
            tags={[suggested, kept]}
            onStaleVideo={onStaleVideo}
            onChanged={onChanged}
          />
        </ToastProvider>
      </MemoryRouter>,
    );
    return { onChanged, onStaleVideo };
  }

  it("shows keep and reject only on the suggested tag", () => {
    renderTags();
    expect(
      screen.getByRole("button", { name: "Keep cat (suggested by auto-tagging)" }),
    ).toBeTruthy();
    expect(
      screen.getByRole("button", { name: "Reject cat for this video" }),
    ).toBeTruthy();
    expect(
      screen.queryByRole("button", { name: "Remove cat from this video" }),
    ).toBeNull();
    expect(
      screen.getByRole("button", { name: "Remove dog from this video" }),
    ).toBeTruthy();
    expect(screen.queryByRole("button", { name: /Keep dog/ })).toBeNull();
  });

  it("keeping a suggestion confirms it and refreshes the video", async () => {
    const user = userEvent.setup();
    const { onChanged } = renderTags();
    await user.click(
      screen.getByRole("button", { name: "Keep cat (suggested by auto-tagging)" }),
    );
    await waitFor(() => expect(onChanged).toHaveBeenCalled());
    const call = fetchMock.mock.calls.find(([input]) =>
      String(input).endsWith("/confirm"),
    );
    expect(String(call?.[0])).toBe("/api/videos/7/tags/5/confirm");
    expect(screen.queryByRole("button", { name: /Keep cat/ })).toBeNull();
    expect(
      screen.getByRole("button", { name: "Remove cat from this video" }),
    ).toBeTruthy();
  });

  it("rejecting a suggestion removes the chip", async () => {
    const user = userEvent.setup();
    const { onChanged } = renderTags();
    await user.click(screen.getByRole("button", { name: "Reject cat for this video" }));
    await waitFor(() => expect(onChanged).toHaveBeenCalled());
    const call = fetchMock.mock.calls.find(([input]) =>
      String(input).endsWith("/reject"),
    );
    expect(String(call?.[0])).toBe("/api/videos/7/tags/5/reject");
    expect(screen.queryByRole("link", { name: /cat/ })).toBeNull();
  });

  it("reloads the video when the suggestion was settled elsewhere", async () => {
    settleStatus = 404;
    const user = userEvent.setup();
    const { onStaleVideo } = renderTags();
    await user.click(screen.getByRole("button", { name: "Reject cat for this video" }));
    await waitFor(() => expect(onStaleVideo).toHaveBeenCalled());
  });
});
