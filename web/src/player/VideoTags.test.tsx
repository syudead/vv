import { act, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { VideoTag } from "../api/client";
import type { Tag } from "../api/tags";
import { __resetTagsForTest, refreshTags } from "../api/tags";
import { ToastProvider } from "../ui/Toast";
import VideoTags from "./VideoTags";

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function tag(overrides: Partial<Tag> & { id: number; name: string }): Tag {
  return {
    synonyms: [],
    videoCount: 0,
    tentative: false,
    createdAt: "2026-01-01T00:00:00Z",
    ...overrides,
  };
}

/** server はタグの一覧と付け外しの経路だけを扱う偽のサーバーである。 */
const server = {
  tags: [] as Tag[],
  attachDelay: null as (() => void) | null,
  detachFails: false,
};

/**
 * holdGetIndices を GET /api/tags の何回目（1始まり）を止めるかの集合に
 * 設定すると、その回だけ release を呼ぶまで応答しない。応答の中身は、要求を
 * 受けた時点の `server.tags` を写し取って持つ（release を呼んだ時点の
 * `server.tags` ではない。tags.ts の generation ガードのテストで、追い越された
 * 古い取得の応答が「その要求を送った時点でのサーバーの状態」を持つように
 * するため）。
 */
let holdGetIndices: Set<number> | null = null;
let getCallCount = 0;
const heldGetReleases: (() => void)[] = [];

function install() {
  const fetchMock = vi.fn<typeof fetch>();
  fetchMock.mockImplementation((input, init) => {
    const url = String(input);
    const method = init?.method ?? "GET";
    if (url === "/api/tags" && method === "GET") {
      getCallCount += 1;
      const index = getCallCount;
      const snapshot = [...server.tags];
      if (holdGetIndices?.has(index) === true) {
        return new Promise((resolve) => {
          heldGetReleases.push(() => resolve(jsonResponse({ items: snapshot })));
        });
      }
      return Promise.resolve(jsonResponse({ items: snapshot }));
    }
    if (url === "/api/video-tags" && method === "POST") {
      const body = JSON.parse(String(init?.body)) as {
        videoIds: number[];
        action: "add" | "remove";
        tag: { id: number } | { name: string };
      };
      const requestedTag = body.tag;
      const resolveNow = () => {
        if (body.action === "remove" && server.detachFails) {
          return jsonResponse({ code: "internal", message: "失敗しました" }, 500);
        }
        let resolvedTag: { id: number; name: string; tentative: boolean };
        if ("id" in requestedTag) {
          const found = server.tags.find((t) => t.id === requestedTag.id);
          if (found === undefined) {
            return jsonResponse(
              { code: "tag_not_found", message: "タグはもうありません" },
              409,
            );
          }
          resolvedTag = { id: found.id, name: found.name, tentative: found.tentative };
        } else {
          const found = server.tags.find(
            (t) => t.name === requestedTag.name || t.synonyms.includes(requestedTag.name),
          );
          if (found !== undefined) {
            resolvedTag = { id: found.id, name: found.name, tentative: found.tentative };
          } else {
            const created = tag({
              id: server.tags.length + 100,
              name: requestedTag.name,
            });
            server.tags.push(created);
            resolvedTag = { id: created.id, name: created.name, tentative: false };
          }
        }
        return jsonResponse({ tag: resolvedTag, applied: body.videoIds.length });
      };
      if (server.attachDelay !== null) {
        return new Promise((resolve) => {
          server.attachDelay = () => resolve(resolveNow());
        });
      }
      return Promise.resolve(resolveNow());
    }
    throw new Error(`想定しない要求: ${method} ${url}`);
  });
  vi.stubGlobal("fetch", fetchMock);
  return fetchMock;
}

function renderTags(videoId: number, tags: VideoTag[], onStaleVideo = vi.fn()) {
  return render(
    <MemoryRouter>
      <ToastProvider>
        <VideoTags videoId={videoId} tags={tags} onStaleVideo={onStaleVideo} />
      </ToastProvider>
    </MemoryRouter>,
  );
}

function addInput() {
  return screen.getByRole("combobox", { name: "Add tag" });
}

beforeEach(() => {
  __resetTagsForTest();
  server.tags = [
    tag({ id: 1, name: "旅行" }),
    tag({ id: 2, name: "Anime", synonyms: ["アニメ"], videoCount: 3 }),
    tag({ id: 3, name: "Drama" }),
  ];
  server.attachDelay = null;
  server.detachFails = false;
  holdGetIndices = null;
  getCallCount = 0;
  heldGetReleases.length = 0;
});

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("VideoTags", () => {
  it("新しい名前を入力して確定すると、その動画にタグが付いて表示される（受け入れ条件1）", async () => {
    const user = userEvent.setup();
    install();
    renderTags(7, []);

    await user.click(addInput());
    await user.type(addInput(), "新しいタグ");
    await user.keyboard("{Enter}");

    expect(await screen.findByTitle("新しいタグ")).toBeDefined();
    expect((addInput() as HTMLInputElement).value).toBe("");
  });

  it("作ったタグは別の動画の候補にも出る（受け入れ条件1）", async () => {
    const user = userEvent.setup();
    install();
    const { unmount } = renderTags(7, []);
    await user.click(addInput());
    await user.type(addInput(), "新規作成タグ");
    await user.keyboard("{Enter}");
    await screen.findByTitle("新規作成タグ");
    unmount();

    renderTags(8, []);
    await user.click(addInput());
    expect(await screen.findByRole("option", { name: /新規作成タグ/ })).toBeDefined();
  });

  it("外すボタンを押すとタグが消える。読み上げ名は「<名>をこの動画から外す」（受け入れ条件2）", async () => {
    install();
    renderTags(7, [
      { id: 1, name: "旅行", manual: true, fromFolder: false, tentative: false },
    ]);

    const removeButton = await screen.findByRole("button", {
      name: "Remove 旅行 from this video",
    });
    fireEvent.click(removeButton);
    expect((removeButton as HTMLButtonElement).disabled).toBe(true);
    await waitFor(() => expect(screen.queryByTitle("旅行")).toBeNull());
  });

  it("タグの名前は /?tag=<id> へのリンクで、読み上げ名は「<名>で絞り込む」（issue 269）", async () => {
    install();
    renderTags(7, [
      { id: 1, name: "旅行", manual: true, fromFolder: false, tentative: false },
    ]);

    const link = await screen.findByRole("link", { name: "Filter by 旅行" });
    expect(link.getAttribute("href")).toBe("/?tag=1");
  });

  // (7): 外せなかったときは、次や前のチップではなく、そのチップ自身の × へ
  // フォーカスを戻す。
  it("外すのに失敗したら、そのチップの×へフォーカスを戻す", async () => {
    install();
    server.detachFails = true;
    renderTags(7, [
      { id: 1, name: "旅行", manual: true, fromFolder: false, tentative: false },
      { id: 2, name: "Anime", manual: true, fromFolder: false, tentative: false },
    ]);

    const removeButton = await screen.findByRole("button", {
      name: "Remove 旅行 from this video",
    });
    fireEvent.click(removeButton);
    expect((removeButton as HTMLButtonElement).disabled).toBe(true);

    await screen.findByText(
      "Couldn't remove the tag: Something went wrong on the server.",
    );
    await waitFor(() => expect((removeButton as HTMLButtonElement).disabled).toBe(false));
    expect(document.activeElement).toBe(removeButton);
    expect(screen.getByTitle("旅行")).toBeDefined();
  });

  it("外す応答を受けてタグが消えるまで、フォーカスを次のチップへ移さない", async () => {
    install();
    server.attachDelay = () => undefined;
    renderTags(7, [
      { id: 1, name: "旅行", manual: true, fromFolder: false, tentative: false },
      { id: 2, name: "Anime", manual: true, fromFolder: false, tentative: false },
    ]);

    const removeButton = await screen.findByRole("button", {
      name: "Remove 旅行 from this video",
    });
    const nextButton = screen.getByRole("button", {
      name: "Remove Anime from this video",
    });
    fireEvent.click(removeButton);
    await waitFor(() => expect((removeButton as HTMLButtonElement).disabled).toBe(true));
    expect(document.activeElement).not.toBe(nextButton);

    act(() => server.attachDelay?.());
    await waitFor(() => expect(screen.queryByTitle("旅行")).toBeNull());
    expect(document.activeElement).toBe(nextButton);
  });

  // フォルダ名からも付いているタグは、手で付けた分を外してもチップが残る。
  // チップが消えるのを待たず、外れた時点でフォーカスを次のチップへ移す。
  it("フォルダ名からも付いているタグを外すと、チップが残っても次のチップへフォーカスを移す", async () => {
    install();
    server.attachDelay = () => undefined;
    renderTags(7, [
      { id: 2, name: "Anime", manual: true, fromFolder: true, tentative: false },
      { id: 1, name: "旅行", manual: true, fromFolder: false, tentative: false },
    ]);

    const removeButton = await screen.findByRole("button", {
      name: "Remove Anime from this video",
    });
    const nextButton = screen.getByRole("button", {
      name: "Remove 旅行 from this video",
    });
    fireEvent.click(removeButton);
    await waitFor(() => expect((removeButton as HTMLButtonElement).disabled).toBe(true));
    expect(document.activeElement).not.toBe(nextButton);

    act(() => server.attachDelay?.());
    await waitFor(() => expect(document.activeElement).toBe(nextButton));
    expect(screen.getByTitle("Anime")).toBeDefined();
  });

  // 017 の ui-design.md「Folder-derived tag chip」・受け入れ条件 6・9。
  // 受け入れ条件 4・5（specs/031-tentative-tags/ui-design.md「Tentative mark」
  // 「Video page」）: 仮のタグは名前の部分（Link）の中で名前の後ろに破線の丸を
  // 置く。縦線と × は今のまま。
  describe("仮のタグ", () => {
    function mark(el: Element): Element | null {
      return el.querySelector("svg.lucide-circle-dashed");
    }

    it("名前の後ろに目印を置き、面・高さ・× は確定したタグと同じ", async () => {
      install();
      renderTags(7, [
        { id: 1, name: "高画質", manual: true, fromFolder: false, tentative: true },
        { id: 2, name: "旅行", manual: true, fromFolder: false, tentative: false },
      ]);

      const link = await screen.findByRole("link", {
        name: "Filter by 高画質 (tentative)",
      });
      expect(link.getAttribute("href")).toBe("/?tag=1");
      const icon = mark(link);
      expect(icon).not.toBeNull();
      expect(icon?.getAttribute("aria-hidden")).toBe("true");
      expect(icon?.getAttribute("class")).toContain("size-3");
      expect(icon?.getAttribute("class")).toContain("text-muted-foreground");
      expect(link.firstElementChild?.textContent).toBe("高画質");
      expect(link.firstElementChild?.className).toContain("truncate");
      expect(link.lastElementChild).toBe(icon);

      const tentativeChip = screen.getByTitle("高画質");
      const confirmedChip = screen.getByTitle("旅行");
      for (const chip of [tentativeChip, confirmedChip]) {
        expect(chip.className).toContain("bg-secondary");
        expect(chip.className).toContain("h-6");
        expect(chip.className).toContain("text-secondary-foreground");
      }
      expect(
        screen.getByRole("button", { name: "Remove 高画質 from this video" }),
      ).toBeDefined();

      // 確定したタグは今と同じ（目印も子要素も足さない）。
      const confirmedLink = screen.getByRole("link", { name: "Filter by 旅行" });
      expect(confirmedLink.querySelector("svg")).toBeNull();
      expect(confirmedLink.children).toHaveLength(0);
      expect(confirmedLink.className).toContain("truncate");
    });

    it("× で仮のタグを今と同じに外せる", async () => {
      install();
      renderTags(7, [
        { id: 1, name: "高画質", manual: true, fromFolder: false, tentative: true },
      ]);
      fireEvent.click(
        await screen.findByRole("button", { name: "Remove 高画質 from this video" }),
      );
      await waitFor(() => expect(screen.queryByTitle("高画質")).toBeNull());
    });

    it("フォルダ由来だけの仮のタグは Folder の目印 → 名前 → 仮の目印の順", async () => {
      install();
      renderTags(7, [
        { id: 4, name: "京都", manual: false, fromFolder: true, tentative: true },
      ]);
      const link = await screen.findByRole("link", {
        name: "Filter by 京都 (from the folder name, tentative)",
      });
      const children = Array.from(link.children);
      expect(children).toHaveLength(3);
      expect(children[0]?.getAttribute("class")).toContain("lucide-folder");
      expect(children[1]?.textContent).toBe("京都");
      expect(children[2]?.getAttribute("class")).toContain("lucide-circle-dashed");
      expect(link.className).toContain("border-dashed");
    });

    it("候補の仮のタグを選ぶと目印付きで並び、新しく作ったタグには目印が出ない", async () => {
      const user = userEvent.setup();
      install();
      server.tags.push(tag({ id: 9, name: "高画質", tentative: true }));
      renderTags(7, []);

      await user.click(addInput());
      await user.type(addInput(), "高画質");
      await user.click(await screen.findByRole("option", { name: /高画質/ }));
      const link = await screen.findByRole("link", {
        name: "Filter by 高画質 (tentative)",
      });
      expect(mark(link)).not.toBeNull();

      await user.type(addInput(), "新しい名前");
      await user.keyboard("{Enter}");
      const created = await screen.findByRole("link", { name: "Filter by 新しい名前" });
      expect(created.querySelector("svg")).toBeNull();
    });
  });

  describe("フォルダ由来のタグ", () => {
    it("フォルダ由来だけのタグは×を出さず、破線の形のリンクで出す（受け入れ条件6）", async () => {
      install();
      renderTags(7, [
        { id: 4, name: "京都", manual: false, fromFolder: true, tentative: false },
        { id: 1, name: "旅行", manual: true, fromFolder: false, tentative: false },
      ]);

      const link = await screen.findByRole("link", {
        name: "Filter by 京都 (from the folder name)",
      });
      expect(link.getAttribute("href")).toBe("/?tag=4");
      expect(link.className).toContain("border-dashed");
      expect(link.className).not.toContain("bg-secondary");
      expect(link.querySelector("svg[aria-hidden='true']")).not.toBeNull();
      expect(
        screen.queryByRole("button", { name: "Remove 京都 from this video" }),
      ).toBeNull();

      // 手で付けたタグは今の形（面あり・×あり）。大きさは同じ h-6・text-xs。
      const manualChip = screen.getByTitle("旅行");
      expect(manualChip.className).toContain("bg-secondary");
      expect(
        screen.getByRole("button", { name: "Remove 旅行 from this video" }),
      ).toBeDefined();
      for (const chip of [link, manualChip]) {
        expect(chip.className).toContain("h-6");
        expect(chip.className).toContain("text-xs");
      }
    });

    it("両方から付いたタグの×は手の分だけを外し、同じ位置で破線の形に変わる（受け入れ条件9）", async () => {
      install();
      server.attachDelay = () => undefined;
      renderTags(7, [
        { id: 2, name: "Anime", manual: true, fromFolder: false, tentative: false },
        { id: 1, name: "旅行", manual: true, fromFolder: true, tentative: false },
        { id: 3, name: "Drama", manual: true, fromFolder: false, tentative: false },
      ]);

      fireEvent.click(
        await screen.findByRole("button", { name: "Remove 旅行 from this video" }),
      );
      // 応答を受けるまでは今の形のまま。
      expect(
        screen.queryByRole("link", { name: /旅行.*from the folder name/ }),
      ).toBeNull();

      act(() => server.attachDelay?.());
      const link = await screen.findByRole("link", {
        name: "Filter by 旅行 (from the folder name)",
      });
      expect(link.className).toContain("border-dashed");
      expect(
        screen.queryByRole("button", { name: "Remove 旅行 from this video" }),
      ).toBeNull();
      const list = screen.getAllByRole("listitem");
      expect(list.slice(0, 3).map((item) => item.textContent)).toEqual([
        "Anime",
        "旅行",
        "Drama",
      ]);
      expect(document.activeElement).toBe(
        screen.getByRole("button", { name: "Remove Drama from this video" }),
      );
    });

    it("外した後のフォーカスは破線のチップを飛ばして次の×へ移る", async () => {
      install();
      renderTags(7, [
        { id: 2, name: "Anime", manual: true, fromFolder: false, tentative: false },
        { id: 4, name: "京都", manual: false, fromFolder: true, tentative: false },
        { id: 1, name: "旅行", manual: true, fromFolder: false, tentative: false },
      ]);

      fireEvent.click(
        await screen.findByRole("button", { name: "Remove Anime from this video" }),
      );
      await waitFor(() => expect(screen.queryByTitle("Anime")).toBeNull());
      expect(document.activeElement).toBe(
        screen.getByRole("button", { name: "Remove 旅行 from this video" }),
      );
    });

    it("次にも前にも×が無ければ、破線のチップを飛ばして「タグを追加」の入力へ移る", async () => {
      install();
      renderTags(7, [
        { id: 4, name: "京都", manual: false, fromFolder: true, tentative: false },
        { id: 1, name: "旅行", manual: true, fromFolder: false, tentative: false },
        { id: 5, name: "夏", manual: false, fromFolder: true, tentative: false },
      ]);

      fireEvent.click(
        await screen.findByRole("button", { name: "Remove 旅行 from this video" }),
      );
      await waitFor(() => expect(screen.queryByTitle("旅行")).toBeNull());
      expect(document.activeElement).toBe(addInput());
    });

    it("フォルダ由来だけのタグは「タグを追加」の候補に出し、確定すると面のある形に変わる", async () => {
      const user = userEvent.setup();
      install();
      renderTags(7, [
        { id: 1, name: "旅行", manual: false, fromFolder: true, tentative: false },
        { id: 3, name: "Drama", manual: true, fromFolder: false, tentative: false },
      ]);

      await user.click(addInput());
      await screen.findByRole("option", { name: /旅行/ });
      expect(screen.queryByRole("option", { name: /Drama/ })).toBeNull();
      await user.click(screen.getByRole("option", { name: /旅行/ }));

      expect(
        await screen.findByRole("button", { name: "Remove 旅行 from this video" }),
      ).toBeDefined();
      expect(screen.getByTitle("旅行").className).toContain("bg-secondary");
      expect(screen.queryByRole("link", { name: /from the folder name/ })).toBeNull();
    });
  });

  it("応答を待つ間に打った次の名前は、先の付与の成功で消さない", async () => {
    const user = userEvent.setup();
    install();
    server.attachDelay = () => undefined;
    renderTags(7, []);

    const input = addInput();
    await user.click(input);
    await user.type(input, "旅行");
    await user.keyboard("{Enter}");
    fireEvent.change(input, { target: { value: "料理" } });

    act(() => server.attachDelay?.());
    expect(await screen.findByTitle("旅行")).toBeDefined();
    expect((input as HTMLInputElement).value).toBe("料理");
  });

  it("付けた後に届いた古い動画の情報では、付けたタグを消さない", async () => {
    const user = userEvent.setup();
    install();
    const onStaleVideo = vi.fn();
    const view = render(
      <MemoryRouter>
        <ToastProvider>
          <VideoTags videoId={7} tags={[]} onStaleVideo={onStaleVideo} />
        </ToastProvider>
      </MemoryRouter>,
    );
    const rerenderWith = (tags: VideoTag[]) =>
      view.rerender(
        <MemoryRouter>
          <ToastProvider>
            <VideoTags videoId={7} tags={tags} onStaleVideo={onStaleVideo} />
          </ToastProvider>
        </MemoryRouter>,
      );

    const input = addInput();
    await user.click(input);
    await user.type(input, "旅行");
    await user.keyboard("{Enter}");
    expect(await screen.findByTitle("旅行")).toBeDefined();

    // 付ける前に始まった取り直しが、タグの無い情報を後から届ける。
    rerenderWith([]);
    expect(screen.getByTitle("旅行")).toBeDefined();

    // 付与を映した情報が届いた後は、その後の情報に従う（別の画面で外された）。
    rerenderWith([
      { id: 1, name: "旅行", manual: true, fromFolder: false, tentative: false },
    ]);
    rerenderWith([]);
    await waitFor(() => expect(screen.queryByTitle("旅行")).toBeNull());
  });

  // VideoPage は videoId ごとに VideoTags を作り直すとは限らず、同じ
  // インスタンスを次の動画にも使い回すことがある（key を付けなければ React
  // 自身は作り直さない）。そのとき前の動画で重ねた付け外しを次の動画へ
  // 持ち越してはならない（Devin の指摘1）。
  it("videoId が変わると、前の動画で重ねた付け外しを持ち越さない（Devinの指摘1）", async () => {
    const user = userEvent.setup();
    install();
    const view = render(
      <MemoryRouter>
        <ToastProvider>
          <VideoTags videoId={7} tags={[]} onStaleVideo={vi.fn()} />
        </ToastProvider>
      </MemoryRouter>,
    );
    const rerenderWith = (videoId: number, tags: VideoTag[]) =>
      view.rerender(
        <MemoryRouter>
          <ToastProvider>
            <VideoTags videoId={videoId} tags={tags} onStaleVideo={vi.fn()} />
          </ToastProvider>
        </MemoryRouter>,
      );

    const input = addInput();
    await user.click(input);
    await user.type(input, "旅行");
    await user.keyboard("{Enter}");
    expect(await screen.findByTitle("旅行")).toBeDefined();

    // 同じ VideoTags のまま、次の動画（タグの無い動画8）へ移る。
    rerenderWith(8, []);
    expect(screen.queryByTitle("旅行")).toBeNull();

    // 動画8自身のタグの付け外しは、そのまま重なる。
    await user.click(addInput());
    await user.type(addInput(), "Anime");
    await user.keyboard("{Enter}");
    expect(await screen.findByTitle("Anime")).toBeDefined();

    // 動画7へ戻っても、動画8で付けた「Anime」は映らない。
    rerenderWith(7, []);
    expect(screen.queryByTitle("Anime")).toBeNull();
  });

  // 重ねた付け外しは、そのタグ自体が別画面での削除・統合で共有の一覧から
  // 消えていれば蘇らせない（Devin の指摘1）。
  it("重ねたタグが共有の一覧から消えていれば蘇らせない（削除・統合。Devinの指摘1）", async () => {
    const user = userEvent.setup();
    install();
    renderTags(7, []);

    const input = addInput();
    await user.click(input);
    await user.type(input, "旅行");
    await user.keyboard("{Enter}");
    expect(await screen.findByTitle("旅行")).toBeDefined();

    // 別の画面で「旅行」が削除された（統合で吸収された場合も同じく消える）。
    server.tags = server.tags.filter((t) => t.name !== "旅行");
    await act(() => refreshTags());

    await waitFor(() => expect(screen.queryByTitle("旅行")).toBeNull());
  });

  // tags.ts の generation ガードは、追い越された取得が「まだ何も反映して
  // いない自分の応答」をそのまま返してはならない（held がまだ空のうちは
  // 特に、それがそのまま漏れる）。初回の GET が保留の間に名前で新しいタグを
  // 付け、その古い（タグを含まない）GET があとから届いても、チップは
  // 消えたまま戻らなくなってはいけない（Devinの指摘）。
  it("初回取得が保留の間に名前で付けたタグは、後から届く古い一覧に消されない（Devinの指摘）", async () => {
    const user = userEvent.setup();
    install();
    // マウントの初回 GET（1回目）と、作成成功後の afterTagCreated の取り直し
    // （2回目）の両方を止める。
    holdGetIndices = new Set([1, 2]);
    renderTags(7, []);
    await waitFor(() => expect(heldGetReleases).toHaveLength(1));

    // 初回 GET が保留（＝ tags.ts の held がまだ空）の間に、新しい名前で
    // タグを付ける。POST 自体は止めていないので、その場で反映される。
    const input = addInput();
    await user.click(input);
    await user.type(input, "新しいタグ");
    await user.keyboard("{Enter}");
    const chip = await screen.findByTitle("新しいタグ");
    await waitFor(() => expect(heldGetReleases).toHaveLength(2));

    // 1回目（作成より前に始まった、タグを含まない）GET が先に届く。
    heldGetReleases[0]!();
    await act(async () => {
      await Promise.resolve();
    });
    expect(screen.getByTitle("新しいタグ")).toBe(chip);

    // 2回目（作成を含む、最新の）GET が届いても、引き続き出ている。
    heldGetReleases[1]!();
    await waitFor(() => expect(screen.getByTitle("新しいタグ")).toBeDefined());
  });

  it("矢印キーで候補を選び、Enter で付けられる（キーボードだけ）", async () => {
    const user = userEvent.setup();
    install();
    renderTags(7, []);

    await user.click(addInput());
    // 候補は名前の自然順（Anime・Drama・旅行）。3 回下矢印で「旅行」を選ぶ。
    await screen.findByRole("option", { name: /旅行/ });
    await user.keyboard("{ArrowDown}{ArrowDown}{ArrowDown}{Enter}");

    expect(await screen.findByTitle("旅行")).toBeDefined();
  });

  // (2): 上矢印は先頭の行で止まる（ui-design.md「Combobox」端で止まる）。
  it("先頭の候補で上矢印を押しても先頭のまま", async () => {
    const user = userEvent.setup();
    install();
    renderTags(7, []);

    await user.click(addInput());
    const first = await screen.findByRole("option", { name: /Anime/ });
    await user.keyboard("{ArrowDown}{ArrowUp}{ArrowUp}");
    expect(first.getAttribute("aria-selected")).toBe("true");
    await user.keyboard("{Enter}");
    expect(await screen.findByTitle("Anime")).toBeDefined();
  });

  it("`anime`を入力すると、シノニム未登録なら別タグとして作られる（受け入れ条件6）", async () => {
    const user = userEvent.setup();
    install();
    renderTags(7, []);

    await user.click(addInput());
    await user.type(addInput(), "anime");
    await user.keyboard("{Enter}");

    const chip = await screen.findByTitle("anime");
    expect(chip).toBeDefined();
    expect(screen.queryByTitle("Anime")).toBeNull();
  });

  it("シノニム登録済みの名前を入力すると、元のタグが付く（受け入れ条件13）", async () => {
    const user = userEvent.setup();
    install();
    renderTags(7, []);

    await user.click(addInput());
    await user.type(addInput(), "アニメ");
    await user.keyboard("{Enter}");

    expect(await screen.findByTitle("Anime")).toBeDefined();
    expect(screen.queryByTitle("アニメ")).toBeNull();
  });

  it("空白だけの入力はEnterで確定できず、理由が出る（受け入れ条件6）", async () => {
    const user = userEvent.setup();
    install();
    renderTags(7, []);

    await user.click(addInput());
    await user.type(addInput(), "   ");
    await user.keyboard("{Enter}");

    expect(await screen.findByText("Enter a name")).toBeDefined();
  });

  it("改行を含む貼り付けは取り込まず、理由が出る（受け入れ条件6）", async () => {
    install();
    renderTags(7, []);

    const input = addInput();
    fireEvent.focus(input);
    const pasteEvent = Object.assign(
      new Event("paste", { bubbles: true, cancelable: true }),
      {
        clipboardData: { getData: () => "旅行\n2024" },
      },
    );
    fireEvent(input, pasteEvent);

    expect(await screen.findByText("Line breaks and tabs aren't allowed")).toBeDefined();
    expect((input as HTMLInputElement).value).toBe("");
  });

  it("101文字の名前は確定できず、文字数の理由が出る（受け入れ条件6）", async () => {
    const user = userEvent.setup();
    install();
    renderTags(7, []);
    const longName = "あ".repeat(101);

    await user.click(addInput());
    await user.type(addInput(), longName);

    expect(
      await screen.findByText("Use 100 characters or fewer (currently 101)"),
    ).toBeDefined();
    await user.keyboard("{Enter}");
    expect(screen.queryByTitle(longName)).toBeNull();
  });

  it("送信中は入力が送信中の表示になり、Enterを受けない", async () => {
    const user = userEvent.setup();
    install();
    server.attachDelay = () => undefined;
    renderTags(7, []);

    await user.click(addInput());
    await user.type(addInput(), "遅いタグ");
    await user.keyboard("{Enter}");

    await waitFor(() => expect(addInput().getAttribute("aria-busy")).toBe("true"));
    await user.keyboard("{Enter}");
    expect(screen.queryByTitle("遅いタグ")).toBeNull();

    await act(async () => {
      server.attachDelay?.();
      await Promise.resolve();
    });
    expect(await screen.findByTitle("遅いタグ")).toBeDefined();
  });

  it("tag_not_foundのときはトーストを出し、この動画を取り直す", async () => {
    install();
    const onStaleVideo = vi.fn();
    renderTags(
      7,
      [
        {
          id: 99,
          name: "もう無いタグ",
          manual: true,
          fromFolder: false,
          tentative: false,
        },
      ],
      onStaleVideo,
    );

    const removeButton = await screen.findByRole("button", {
      name: "Remove もう無いタグ from this video",
    });
    fireEvent.click(removeButton);

    expect(
      await screen.findByText(
        'The tag "もう無いタグ" no longer exists, so the tags were reloaded',
      ),
    ).toBeDefined();
    expect(onStaleVideo).toHaveBeenCalled();
  });

  it("すでに付いているタグは候補に出ない", async () => {
    const user = userEvent.setup();
    install();
    renderTags(7, [
      { id: 1, name: "旅行", manual: true, fromFolder: false, tentative: false },
    ]);

    await user.click(addInput());
    await screen.findByRole("option", { name: /Anime/ });
    expect(screen.queryByRole("option", { name: /旅行/ })).toBeNull();
  });

  it("シノニムで当たった候補には「シノニム: 」を添える", async () => {
    const user = userEvent.setup();
    install();
    renderTags(7, []);

    await user.click(addInput());
    await user.type(addInput(), "アニメ");

    const option = await screen.findByRole("option", { name: /Anime/ });
    expect(within(option).getByText("Synonym: アニメ")).toBeDefined();
  });

  // B1: 一覧が開いていれば1回目のEscでそれだけを閉じ、すでに閉じていれば
  // 2回目のEscで入力を空にする（ui-design.md「Add input」）。
  it("一覧を閉じてからのEscで入力を空にする", async () => {
    const user = userEvent.setup();
    install();
    renderTags(7, []);

    const input = addInput();
    await user.click(input);
    await user.type(input, "途中まで");
    await screen.findByRole("option", { name: /Create "/ });

    await user.keyboard("{Escape}");
    expect(screen.queryByRole("option")).toBeNull();
    expect((input as HTMLInputElement).value).toBe("途中まで");

    await user.keyboard("{Escape}");
    expect((input as HTMLInputElement).value).toBe("");
  });

  // B2: 101文字・改行貼り付け直後・送信中は、Enterだけでなくクリックでも
  // 確定を受けない（候補やしずれの行をクリックしてすり抜けさせない）。
  describe("クリックでの確定も、Enterと同じ検査で塞ぐ（B2）", () => {
    it("101文字の名前のときは作成の行をクリックしても作られない", async () => {
      const user = userEvent.setup();
      install();
      renderTags(7, []);
      const longName = "あ".repeat(101);

      await user.click(addInput());
      await user.type(addInput(), longName);
      await screen.findByText("Use 100 characters or fewer (currently 101)");

      const createRow = screen.getByRole("option", { name: /Create "/ });
      expect(createRow.getAttribute("aria-disabled")).toBe("true");
      fireEvent.click(createRow);

      await new Promise((resolve) => setTimeout(resolve, 0));
      expect(screen.queryByTitle(longName)).toBeNull();
    });

    it("改行貼り付け直後は候補の行をクリックしても付かない", async () => {
      install();
      renderTags(7, []);

      const input = addInput();
      fireEvent.focus(input);
      await screen.findByRole("option", { name: /旅行/ });
      const pasteEvent = Object.assign(
        new Event("paste", { bubbles: true, cancelable: true }),
        { clipboardData: { getData: () => "旅行\n2024" } },
      );
      fireEvent(input, pasteEvent);
      await screen.findByText("Line breaks and tabs aren't allowed");

      const option = screen.getByRole("option", { name: /旅行/ });
      expect(option.getAttribute("aria-disabled")).toBe("true");
      fireEvent.click(option);

      await new Promise((resolve) => setTimeout(resolve, 0));
      expect(screen.queryByTitle("旅行")).toBeNull();
    });

    it("送信中は候補の行をクリックしても、重ねて付かない", async () => {
      const user = userEvent.setup();
      install();
      server.attachDelay = () => undefined;
      renderTags(7, []);

      await user.click(addInput());
      await user.type(addInput(), "遅いタグ");
      await user.keyboard("{Enter}");
      await waitFor(() => expect(addInput().getAttribute("aria-busy")).toBe("true"));

      // 値が「遅いタグ」のままなので、候補は作成の行だけになる。それをクリックしても
      // 二重には送らない。
      const option = screen.getByRole("option", { name: /Create "/ });
      expect(option.getAttribute("aria-disabled")).toBe("true");
      const fetchCallsBefore = server.tags.length;
      fireEvent.click(option);
      expect(server.tags.length).toBe(fetchCallsBefore);

      await act(async () => {
        server.attachDelay?.();
        await Promise.resolve();
      });
      expect(await screen.findByTitle("遅いタグ")).toBeDefined();
      expect(screen.queryByTitle("Anime")).toBeNull();
    });
  });

  // B3: IME の変換中の Enter は確定にしない（web/src/player/keyboard.ts の
  // isComposing の扱いと同じ）。
  it("IME変換中のEnterは確定せず、確定後のEnterは効く（B3）", async () => {
    const user = userEvent.setup();
    install();
    renderTags(7, []);

    const input = addInput();
    await user.click(input);
    await user.type(input, "旅行");
    await screen.findByRole("option", { name: /旅行/ });

    fireEvent.keyDown(input, { key: "Enter", isComposing: true });
    await new Promise((resolve) => setTimeout(resolve, 0));
    expect(screen.queryByTitle("旅行")).toBeNull();

    fireEvent.keyDown(input, { key: "Enter", isComposing: false });
    expect(await screen.findByTitle("旅行")).toBeDefined();
  });

  // B4: plan の Structural Decisions 8「画面が開くときに取り直す」。すでに
  // 共有の保持があっても、マウント時に GET /api/tags をもう一度送る。
  it("画面が開くときは、直前に届いた保持はそのまま使い、古い保持なら取り直す（B4・issue 674）", async () => {
    const fetchMock = install();
    const now = vi.spyOn(Date, "now").mockReturnValue(1_000_000);
    const tagCalls = () =>
      fetchMock.mock.calls.filter(([input]) => String(input) === "/api/tags").length;
    try {
      await refreshTags();
      expect(tagCalls()).toBe(1);

      // 届いたばかりの一覧は、開いた部品がそれぞれ取り直さない。
      const first = renderTags(7, []);
      await act(async () => Promise.resolve());
      expect(tagCalls()).toBe(1);
      first.unmount();

      now.mockReturnValue(1_000_000 + 5_000);
      renderTags(7, []);
      await waitFor(() => expect(tagCalls()).toBe(2));
    } finally {
      now.mockRestore();
    }
  });
});
