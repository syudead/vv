import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import FileLocation from "./FileLocation";

function json(body: unknown, status: number): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

describe("FileLocation", () => {
  const fetchMock = vi.fn<typeof fetch>();
  beforeEach(() => vi.stubGlobal("fetch", fetchMock));
  afterEach(() => {
    vi.unstubAllGlobals();
    fetchMock.mockReset();
  });

  it("開けないときは押せる要素にしない", () => {
    render(
      <FileLocation videoId={7} location={{ path: "/media/a/b.mp4", openable: false }} />,
    );
    expect(screen.queryByRole("button")).toBeNull();
    const text = screen.getByTitle("/media/a/b.mp4");
    expect(text.tagName).toBe("P");
    expect(text.className).not.toContain("underline");
    expect(text.textContent).toBe("/media/a/b.mp4");
  });

  it("開けるときはパス全体を 1 つのボタンにし、押すと開く要求を送る", async () => {
    fetchMock.mockResolvedValue(new Response(null, { status: 204 }));
    render(
      <FileLocation videoId={7} location={{ path: "/media/a/b.mp4", openable: true }} />,
    );
    const button = screen.getByRole("button", { name: "ファイルを開く: /media/a/b.mp4" });
    fireEvent.click(button);
    await waitFor(() =>
      expect(fetchMock).toHaveBeenCalledWith(
        "/api/videos/7/open",
        expect.objectContaining({ method: "POST" }),
      ),
    );
    expect(screen.queryByRole("alert")).toBeNull();
  });

  it("file_missing なら理由付き、それ以外なら短い文言を場所の下に出し、次の操作で消す", async () => {
    fetchMock
      .mockResolvedValueOnce(json({ code: "file_missing", message: "無い" }, 409))
      .mockResolvedValueOnce(json({ code: "internal", message: "失敗" }, 500))
      .mockReturnValueOnce(new Promise(() => undefined));
    render(
      <FileLocation videoId={7} location={{ path: "/media/a/b.mp4", openable: true }} />,
    );
    const button = screen.getByRole("button", { name: /ファイルを開く/ });
    fireEvent.click(button);
    expect((await screen.findByRole("alert")).textContent).toBe(
      "開けませんでした: ファイルが見つかりません",
    );
    fireEvent.click(button);
    await waitFor(() =>
      expect(screen.getByRole("alert").textContent).toBe("開けませんでした"),
    );
    fireEvent.click(button);
    expect(screen.queryByRole("alert")).toBeNull();
  });
});
