import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import type { Video } from "../../api/client";
import { formatDateTime } from "../../i18n";
import OverviewPanel, { workSummary } from "./OverviewPanel";

const video: Video = {
  id: 7,
  virtual: false,
  title: "a",
  sizeBytes: 1,
  addedAt: "2026-09-20T12:00:00Z",
  updatedAt: "2026-09-20T12:00:00Z",
  fileCreatedAt: "2026-09-20T12:00:00Z",
  fileModifiedAt: "2026-09-20T12:00:00Z",
  playable: true,
  probeState: "done",
  thumbnailState: "done",
  previewState: "done",
  durationMs: 242_000,
  width: 1920,
  height: 1080,
  progress: { positionMs: 1, completed: false, updatedAt: "2026-10-02T12:00:00Z" },
  workInfo: {
    code: "A-1",
    description: "about",
    dates: [],
    people: [],
    organizations: [],
    urls: [],
    externalIds: [],
  },
  tags: [],
  public: true,
};

function terms(): string[] {
  return screen.getAllByRole("term").map((term) => term.textContent ?? "");
}

describe("OverviewPanel", () => {
  it("読み取りが終わるまでは長さと解像度を出さない", () => {
    render(<OverviewPanel video={{ ...video, probeState: "pending" }} owner />);
    expect(terms()).toEqual(["Added", "Last watched"]);
  });

  it("視聴の記録が無ければ Last watched を出さない", () => {
    render(<OverviewPanel video={{ ...video, progress: undefined }} owner />);
    expect(terms()).toEqual(["Length", "Resolution", "Added"]);
  });

  it("所有者には説明全文を Description の見出しの下に、作品の要約を Work の見出しの下に出し、8 行を超えるときだけ畳む", () => {
    const long = "First paragraph.\n\nSecond paragraph.\nThird line.";
    render(
      <OverviewPanel
        video={{ ...video, workInfo: { ...video.workInfo!, description: long } }}
        owner
      />,
    );
    const description = screen.getByRole("region", { name: "Description" });
    const text = within(description).getByText(/First paragraph/);
    expect(text.textContent).toBe(long);
    expect(text.className).toContain("line-clamp-8");
    expect(within(description).queryByRole("button", { name: "Show more" })).toBeNull();
    expect(
      within(screen.getByRole("region", { name: "Work" })).getByText("A-1"),
    ).toBeDefined();
  });

  it("説明の <br> は改行として出し、ほかのタグは文字のまま出す", () => {
    render(
      <OverviewPanel
        video={{
          ...video,
          workInfo: { ...video.workInfo!, description: "one<br>two<BR />three <b>x</b>" },
        }}
        owner
      />,
    );
    const description = screen.getByRole("region", { name: "Description" });
    expect(within(description).getByText(/^one/).textContent).toBe(
      "one\ntwo\nthree <b>x</b>",
    );
  });

  it("説明が無ければ Work の見出しだけ、要約が無ければ Description の見出しだけを出す", () => {
    const { unmount } = render(
      <OverviewPanel
        video={{ ...video, workInfo: { ...video.workInfo!, description: undefined } }}
        owner
      />,
    );
    expect(screen.queryByRole("region", { name: "Description" })).toBeNull();
    expect(screen.getByRole("region", { name: "Work" })).toBeDefined();
    unmount();

    render(
      <OverviewPanel
        video={{ ...video, workInfo: { ...video.workInfo!, code: undefined } }}
        owner
      />,
    );
    expect(screen.getByRole("region", { name: "Description" })).toBeDefined();
    expect(screen.queryByRole("region", { name: "Work" })).toBeNull();
  });

  it("日付は日付だけを出し、名前と時刻を title に持ち、押すと吹き出しを開いて Esc で閉じる（UI品質）", async () => {
    // Noon keeps the formatted date stable in any time zone.
    const added = new Date(2026, 8, 20, 12, 0).toISOString();
    render(<OverviewPanel video={{ ...video, addedAt: added }} owner />);
    const trigger = screen.getByRole("button", { name: "Added Sep 20, 2026" });
    expect(trigger.textContent).toBe("Sep 20, 2026");
    expect(trigger.title).toBe(`Added ${formatDateTime(added)}`);
    trigger.focus();
    fireEvent.click(trigger);
    const bubble = await screen.findByRole("dialog");
    expect(bubble.textContent).toBe(`Added ${formatDateTime(added)}`);

    fireEvent.keyDown(document.activeElement ?? document.body, { key: "Escape" });
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(document.activeElement).toBe(trigger);
  });

  it("ゲストには説明・作品の要約・Last watched を出さない", () => {
    render(<OverviewPanel video={video} owner={false} />);
    expect(terms()).toEqual(["Length", "Resolution", "Added"]);
    expect(screen.queryByText("about")).toBeNull();
    expect(screen.queryByText("A-1")).toBeNull();
  });
});

describe("workSummary", () => {
  it("出演者を人物ごとに一度だけ並べ、同名の別人は両方残す", () => {
    const performer = (id: number, name: string) => ({
      role: "performer" as const,
      person: { id, name, provisional: false },
      provisional: false,
    });
    const summary = workSummary({
      code: "A-1",
      dates: [],
      organizations: [],
      urls: [],
      externalIds: [],
      people: [performer(1, "Alice"), performer(2, "Alice"), performer(1, "Alice")],
    });
    expect(summary).toBe("A-1 · Alice, Alice");
  });
});
