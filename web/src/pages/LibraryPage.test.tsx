import { act, cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, useNavigate } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { Video, VideoPage } from "../api/client";

/**
 * 一覧の 4 状態と操作の規則
 * （contracts/screen-states.md 1.・3.）。
 *
 * 要点は「帯が常に使える」ことである。通信中も失敗中も、探す・並べ替える・
 * 取り込むが押せないと、利用者は待つか再読み込みするしか手が無くなる（FR-002）。
 * 空の 2 通りを言い分けること（FR-009）と、到達順（FR-003 / FR-020）も、
 * 見た目を見るだけでは確かめられない。
 */

const { listVideos, getCurrentScan, startScan } = vi.hoisted(() => ({
  listVideos: vi.fn(),
  getCurrentScan: vi.fn(),
  startScan: vi.fn(),
}));

vi.mock("../api/client", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../api/client")>()),
  listVideos,
  getCurrentScan,
  startScan,
}));

const { default: LibraryPage } = await import("./LibraryPage");
const { default: AppShell } = await import("../layout/AppShell");

/** searchDebounceMs は LibraryPage の待ち合わせ時間と同じ値である。 */
const searchDebounceMs = 250;

const emptyPage: VideoPage = { items: [], total: 0 };

/** video は一覧に並べる 1 件を作る。 */
function video(id: number, title: string): Video {
  return {
    id,
    title,
    sizeBytes: 1024,
    addedAt: "2026-09-13T00:00:00Z",
    playable: true,
    probeState: "done",
    thumbnailState: "done",
  };
}

/** settle は描画直後の取得を終わらせる。 */
async function settle(): Promise<void> {
  await act(async () => {
    await Promise.resolve();
  });
}

/** enabled は要素が押せる状態かどうかを返す（jest-dom を入れていない）。 */
function enabled(element: HTMLElement): boolean {
  return !(element as HTMLInputElement | HTMLButtonElement | HTMLSelectElement).disabled;
}

/** show は一覧を描く。path で検索語や並び順を与えられる。 */
function show(path = "/") {
  render(
    <MemoryRouter initialEntries={[path]}>
      <LibraryPage />
    </MemoryRouter>,
  );
}

beforeEach(() => {
  vi.useFakeTimers();

  listVideos.mockReset();
  listVideos.mockResolvedValue(emptyPage);
  getCurrentScan.mockReset();
  getCurrentScan.mockResolvedValue(null);
  startScan.mockReset();

  // jsdom は交差の観測を持たない。続きの読み込みはこの検査の対象ではない。
  vi.stubGlobal(
    "IntersectionObserver",
    class {
      observe() {}
      unobserve() {}
      disconnect() {}
      takeRecords() {
        return [];
      }
    },
  );
});

afterEach(() => {
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

describe("LibraryPage の 4 状態（contracts/screen-states.md 1.）", () => {
  it("通信中は骨組みを並べ、そのあいだも帯が押せる", async () => {
    // 応答を止めたままにして「1 ページ目を待っている」状態を作る。
    listVideos.mockReturnValue(new Promise<VideoPage>(() => undefined));
    show();
    await settle();

    expect(screen.getByRole("status", { name: "読み込み中" })).toBeDefined();
    expect(screen.getByText("読み込み中…")).toBeDefined();

    // 帯は状態によらず先に出る。待っているあいだも操作できる。
    expect(enabled(screen.getByPlaceholderText("題名で探す"))).toBe(true);
    expect(enabled(screen.getByLabelText("並び順"))).toBe(true);
    expect(enabled(screen.getByRole("button", { name: "取り込む" }))).toBe(true);
  });

  it("成功したら項目が並ぶ", async () => {
    listVideos.mockResolvedValue({
      items: [video(1, "ねこ.mp4"), video(2, "いぬ.mp4")],
      total: 2,
    } satisfies VideoPage);
    show();
    await settle();

    expect(screen.getByRole("link", { name: "ねこ.mp4" })).toBeDefined();
    expect(screen.getByRole("link", { name: "いぬ.mp4" })).toBeDefined();
  });

  it("失敗しても帯は残り、再試行できる", async () => {
    listVideos.mockRejectedValue(new Error("つながりません"));
    show();
    await settle();

    expect(screen.getByText("一覧を取得できません")).toBeDefined();
    expect(screen.getByRole("button", { name: "再試行" })).toBeDefined();

    // 失敗を 1 枚の画面に差し替えると、検索も取り込みもできなくなる（FR-002）。
    expect(enabled(screen.getByPlaceholderText("題名で探す"))).toBe(true);
    expect(enabled(screen.getByRole("button", { name: "取り込む" }))).toBe(true);
  });

  it("空（蔵書 0）と空（該当 0）は別の文言である", async () => {
    show();
    await settle();
    const library = screen.getByText("動画がまだありません").textContent;
    // 蔵書が 0 のときに検索語を消す入口を出しても、消すものが無い。
    expect(screen.queryByRole("button", { name: "検索語を消す" })).toBeNull();

    cleanup();

    show("/?q=ねこ");
    await settle();
    const matches = screen.getByText("「ねこ」に一致する動画はありません").textContent;

    // 「0 本」とだけ出すと、置き場所が違うのか検索語が悪いのかを区別できない。
    expect(matches).not.toBe(library);
    expect(screen.getByRole("button", { name: "検索語を消す" })).toBeDefined();
  });
});

describe("LibraryPage の件数の文言（FR-008 / FR-021）", () => {
  it("検索の前後で文言が変わり、変化を知らせる領域にある", async () => {
    listVideos.mockResolvedValue({ items: [video(1, "ねこ.mp4")], total: 1 });
    const user = userEvent.setup({ delay: null, advanceTimers: vi.advanceTimersByTime });
    show();
    await settle();

    const count = screen.getByText("1 本");
    expect(count.getAttribute("role")).toBe("status");
    expect(count.getAttribute("aria-live")).toBe("polite");

    await user.type(screen.getByPlaceholderText("題名で探す"), "ねこ");
    await act(async () => {
      vi.advanceTimersByTime(searchDebounceMs);
      await Promise.resolve();
    });

    // 絞られているかどうかを、文言だけで判断できる。
    expect(screen.getByText("「ねこ」に一致 1 本")).toBeDefined();
  });
});

/** GoBack は履歴を 1 つ戻す（検査のためだけの部品）。 */
function GoBack() {
  const navigate = useNavigate();
  return (
    <button type="button" onClick={() => void navigate(-1)}>
      戻る（検査用）
    </button>
  );
}

describe("LibraryPage の戻る／進む（R-403）", () => {
  it("戻ると検索欄も URL の語に戻り、古い語を書き戻さない", async () => {
    const user = userEvent.setup({ delay: null, advanceTimers: vi.advanceTimersByTime });
    render(
      <MemoryRouter initialEntries={["/?q=ねこ", "/?q=いぬ"]} initialIndex={1}>
        <LibraryPage />
        <GoBack />
      </MemoryRouter>,
    );
    await settle();

    const field = () => screen.getByPlaceholderText("題名で探す") as HTMLInputElement;
    expect(field().value).toBe("いぬ");

    await user.click(screen.getByRole("button", { name: "戻る（検査用）" }));
    // 待ち合わせが明けても、古い「いぬ」が URL へ書き戻されない。
    await act(async () => {
      vi.advanceTimersByTime(searchDebounceMs * 2);
      await Promise.resolve();
    });

    expect(field().value).toBe("ねこ");
    expect(screen.getByText("「ねこ」に一致 0 本")).toBeDefined();
  });
});

describe("LibraryPage の到達順（contracts/screen-states.md 3.）", () => {
  it("探す → 並べ替える → 密度 → 取り込む → 一覧の項目 の順に届く", async () => {
    listVideos.mockResolvedValue({
      items: [video(1, "ねこ.mp4"), video(2, "いぬ.mp4")],
      total: 2,
    } satisfies VideoPage);
    const user = userEvent.setup({ delay: null, advanceTimers: vi.advanceTimersByTime });
    show();
    await settle();

    // 密度（US3）は 3 番目に割り込む。帯が項目より先であることと、項目が
    // 並んでいる順であることは変わらない。
    const expected = [
      screen.getByPlaceholderText("題名で探す"),
      screen.getByLabelText("並び順"),
      screen.getByLabelText("表示"),
      screen.getByRole("button", { name: "取り込む" }),
      screen.getByRole("link", { name: "ねこ.mp4" }),
      screen.getByRole("link", { name: "いぬ.mp4" }),
    ];

    for (const element of expected) {
      await user.tab();
      expect(document.activeElement).toBe(element);
    }
  });
});

describe("サイドバーへ公開する件数（T021）", () => {
  /**
   * showInShell は骨格ごと描く。件数はサイドバーの「すべての動画」に出るので、
   * LibraryPage 単体では確かめられない。
   */
  function showInShell(path = "/") {
    render(
      <MemoryRouter initialEntries={[path]}>
        <AppShell>
          <LibraryPage />
        </AppShell>
      </MemoryRouter>,
    );
  }

  /**
   * navCount はサイドバーの「すべての動画」に出ている数字を返す。出ていなければ
   * null を返す。帯の右端の件数とは別の場所を見ている。
   */
  function navCount(): string | null {
    const row = document.querySelector('[data-nav-id="all-videos"]');
    return /\d+/.exec(row?.textContent ?? "")?.[0] ?? null;
  }

  it("読めたら総件数が出る", async () => {
    listVideos.mockResolvedValue({
      items: [video(1, "ねこ.mp4")],
      total: 12,
    } satisfies VideoPage);
    showInShell();
    await settle();

    expect(navCount()).toBe("12");
  });

  it("取得に失敗したら件数を出さない", async () => {
    // useVideos は失敗しても total を書き換えない（初回なら初期値の 0 が残る）。
    // それをそのまま公開すると、一覧がエラーを出している横でサイドバーが件数を
    // 名乗る。読めていないものの数は「まだ分からない」であって 0 ではない。
    listVideos.mockRejectedValue(new Error("つながりません"));
    showInShell();
    await settle();

    expect(screen.getByText("一覧を取得できません")).toBeDefined();
    expect(navCount()).toBeNull();
  });

  it("読めたあとに取り直しが失敗したら件数を取り下げる", async () => {
    // 前の件数が残っているぶん、こちらのほうが嘘が長く見える。検索語を変えて
    // 取り直す場面がこれにあたる。
    listVideos.mockResolvedValue({
      items: [video(1, "ねこ.mp4")],
      total: 12,
    } satisfies VideoPage);
    showInShell();
    await settle();
    expect(navCount()).toBe("12");

    listVideos.mockRejectedValue(new Error("つながりません"));
    await act(async () => {
      await userEvent
        .setup({ advanceTimers: vi.advanceTimersByTime })
        .type(screen.getByPlaceholderText("題名で探す"), "いぬ");
    });
    await act(async () => {
      vi.advanceTimersByTime(searchDebounceMs);
    });
    await settle();

    expect(navCount()).toBeNull();
  });
});
