import { act, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

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
  return { synonyms: [], videoCount: 0, ...overrides };
}

/** server はタグの一覧と付け外しの経路だけを扱う偽のサーバーである。 */
const server = {
  tags: [] as Tag[],
  attachDelay: null as (() => void) | null,
  detachFails: false,
};

function install() {
  const fetchMock = vi.fn<typeof fetch>();
  fetchMock.mockImplementation((input, init) => {
    const url = String(input);
    const method = init?.method ?? "GET";
    if (url === "/api/tags" && method === "GET") {
      return Promise.resolve(jsonResponse({ items: server.tags }));
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
        let resolvedTag: { id: number; name: string };
        if ("id" in requestedTag) {
          const found = server.tags.find((t) => t.id === requestedTag.id);
          if (found === undefined) {
            return jsonResponse(
              { code: "tag_not_found", message: "タグはもうありません" },
              409,
            );
          }
          resolvedTag = { id: found.id, name: found.name };
        } else {
          const found = server.tags.find(
            (t) => t.name === requestedTag.name || t.synonyms.includes(requestedTag.name),
          );
          if (found !== undefined) {
            resolvedTag = { id: found.id, name: found.name };
          } else {
            const created = tag({
              id: server.tags.length + 100,
              name: requestedTag.name,
            });
            server.tags.push(created);
            resolvedTag = { id: created.id, name: created.name };
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

function renderTags(
  videoId: number,
  tags: { id: number; name: string }[],
  onStaleVideo = vi.fn(),
) {
  return render(
    <MemoryRouter>
      <ToastProvider>
        <VideoTags videoId={videoId} tags={tags} onStaleVideo={onStaleVideo} />
      </ToastProvider>
    </MemoryRouter>,
  );
}

function addInput() {
  return screen.getByRole("combobox", { name: "タグを追加" });
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
    renderTags(7, [{ id: 1, name: "旅行" }]);

    const removeButton = await screen.findByRole("button", {
      name: "旅行をこの動画から外す",
    });
    fireEvent.click(removeButton);
    expect((removeButton as HTMLButtonElement).disabled).toBe(true);
    await waitFor(() => expect(screen.queryByTitle("旅行")).toBeNull());
  });

  it("タグの名前は /?tag=<id> へのリンクで、読み上げ名は「<名>で絞り込む」（issue 269）", async () => {
    install();
    renderTags(7, [{ id: 1, name: "旅行" }]);

    const link = await screen.findByRole("link", { name: "旅行で絞り込む" });
    expect(link.getAttribute("href")).toBe("/?tag=1");
  });

  // (7): 外せなかったときは、次や前のチップではなく、そのチップ自身の × へ
  // フォーカスを戻す。
  it("外すのに失敗したら、そのチップの×へフォーカスを戻す", async () => {
    install();
    server.detachFails = true;
    renderTags(7, [
      { id: 1, name: "旅行" },
      { id: 2, name: "Anime" },
    ]);

    const removeButton = await screen.findByRole("button", {
      name: "旅行をこの動画から外す",
    });
    fireEvent.click(removeButton);
    expect((removeButton as HTMLButtonElement).disabled).toBe(true);

    await screen.findByText("タグを外せませんでした");
    await waitFor(() => expect((removeButton as HTMLButtonElement).disabled).toBe(false));
    expect(document.activeElement).toBe(removeButton);
    expect(screen.getByTitle("旅行")).toBeDefined();
  });

  it("外す応答を受けてタグが消えるまで、フォーカスを次のチップへ移さない", async () => {
    install();
    server.attachDelay = () => undefined;
    renderTags(7, [
      { id: 1, name: "旅行" },
      { id: 2, name: "Anime" },
    ]);

    const removeButton = await screen.findByRole("button", {
      name: "旅行をこの動画から外す",
    });
    const nextButton = screen.getByRole("button", { name: "Animeをこの動画から外す" });
    fireEvent.click(removeButton);
    await waitFor(() => expect((removeButton as HTMLButtonElement).disabled).toBe(true));
    expect(document.activeElement).not.toBe(nextButton);

    act(() => server.attachDelay?.());
    await waitFor(() => expect(screen.queryByTitle("旅行")).toBeNull());
    expect(document.activeElement).toBe(nextButton);
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
    const rerenderWith = (tags: { id: number; name: string }[]) =>
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
    rerenderWith([{ id: 1, name: "旅行" }]);
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
    const rerenderWith = (videoId: number, tags: { id: number; name: string }[]) =>
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

    expect(await screen.findByText("名前を入力してください")).toBeDefined();
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

    expect(await screen.findByText("改行やタブは使えません")).toBeDefined();
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
      await screen.findByText("100 文字以内にしてください（今 101 文字）"),
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
    renderTags(7, [{ id: 99, name: "もう無いタグ" }], onStaleVideo);

    const removeButton = await screen.findByRole("button", {
      name: "もう無いタグをこの動画から外す",
    });
    fireEvent.click(removeButton);

    expect(
      await screen.findByText("タグ「もう無いタグ」はもう無いため、一覧を取り直しました"),
    ).toBeDefined();
    expect(onStaleVideo).toHaveBeenCalled();
  });

  it("すでに付いているタグは候補に出ない", async () => {
    const user = userEvent.setup();
    install();
    renderTags(7, [{ id: 1, name: "旅行" }]);

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
    expect(within(option).getByText("シノニム: アニメ")).toBeDefined();
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
    await screen.findByRole("option", { name: /を作成/ });

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
      await screen.findByText("100 文字以内にしてください（今 101 文字）");

      const createRow = screen.getByRole("option", { name: /を作成/ });
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
      await screen.findByText("改行やタブは使えません");

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
      const option = screen.getByRole("option", { name: /を作成/ });
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
  it("画面が開くときは、共有の保持があっても取り直す（B4）", async () => {
    const fetchMock = install();
    await refreshTags();
    const countBefore = fetchMock.mock.calls.filter(
      ([input]) => String(input) === "/api/tags",
    ).length;
    expect(countBefore).toBe(1);

    renderTags(7, []);

    await waitFor(() => {
      const count = fetchMock.mock.calls.filter(
        ([input]) => String(input) === "/api/tags",
      ).length;
      expect(count).toBeGreaterThan(countBefore);
    });
  });
});
