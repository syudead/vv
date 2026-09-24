import { act, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { Tag } from "../api/tags";
import { __resetTagsForTest } from "../api/tags";
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
    <ToastProvider>
      <VideoTags videoId={videoId} tags={tags} onStaleVideo={onStaleVideo} />
    </ToastProvider>,
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

  it("送信中は追加ボタンが送信中の表示になり、Enterを受けない", async () => {
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
});
