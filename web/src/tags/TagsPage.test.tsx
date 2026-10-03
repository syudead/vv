import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { Tag } from "../api/tags";
import { __resetTagsForTest, refreshTags } from "../api/tags";
import { enablePseudoLocale, expectCatalogTextOnly } from "../i18n/pseudo";
import { ToastProvider } from "../ui/Toast";
import { TooltipProvider } from "../ui/Tooltip";
import TagsPage from "./TagsPage";

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(status === 204 ? null : JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

/**
 * takenResponse は実際のサーバーと同じ形の tag_name_taken を返す
 * （contracts/error-api.md §1）。submitted が owner の元の名前なら name_is_tag、
 * そうでなければ name_is_synonym。画面がカタログの文を出す（サーバーの
 * message をそのまま出さない）ことを確かめるため、message は画面に出ない文にする。
 */
function takenResponse(submitted: string, ownerName: string): Response {
  return jsonResponse(
    {
      code: "tag_name_taken",
      reason: submitted === ownerName ? "name_is_tag" : "name_is_synonym",
      tagName: ownerName,
      message: "server-side message",
    },
    409,
  );
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

/** server はタグの管理経路（GET・POST・PATCH・DELETE /api/tags*）を扱う偽のサーバーである。 */
const server = {
  tags: [] as Tag[],
  nextId: 100,
  getCalls: 0,
  /** true にすると、次の GET /api/tags を 500 で失敗させる（N6: 取り直しの失敗）。 */
  failNextGet: false,
  /**
   * true にすると、以後の GET /api/tags をすべて 500 で失敗させ続ける
   * （自分では戻らない）。バックグラウンドの取り直し（`afterTagChanged` の
   * `refreshTags`）が、確かめたいその場の反映を上書きしてしまわないように
   * するためのもの。
   */
  failAllGets: false,
  /** true にすると、次のタグ作成を一般の理由で失敗させる。 */
  failNextCreate: false,
  /** true にすると、次の PATCH /api/tags/{id} を tag_name_taken 以外の理由で失敗させる（N2）。 */
  failNextPatch: false,
  /** true にすると、次の POST /api/tags/{id}/merge を一般の理由で失敗させる（N4）。 */
  failNextMerge: false,
  /** true にすると、次の POST /api/tags/{id}/synonyms を一般の理由で失敗させる（N4）。 */
  failNextSynonymsPost: false,
  /**
   * true にすると、次の POST /api/tags/{id}/synonyms を、実際の所有者に
   * かかわらず tag_merge_required で1回だけ失敗させる（自分で消費して false
   * に戻る）。「確認をとる時点ではもう誰も持っていない名前」を、タイミングを
   * 作らずに再現するための仕掛け（N6b）。
   */
  forceMergeRequiredOnce: false,
  /**
   * 設定すると、次の変更の要求（POST・PATCH・DELETE）をこの応答で1回だけ失敗させる
   * （自分で消費して null に戻る）。API エラーの英語の説明を確かめるためのもの。
   */
  nextError: null as { status: number; body: Record<string, unknown> } | null,
  /** 却下した名前の一覧（GET・DELETE /api/tags/rejected-names）。 */
  rejectedNames: [] as string[],
  /** true にすると、以後の GET /api/tags/rejected-names をすべて 500 で失敗させる。 */
  failRejectedGets: false,
  /** 確定・却下・却下した名前の取り外しの要求の回数。 */
  confirmCalls: 0,
};

/**
 * holdRejectedGets を true にした間の GET /api/tags/rejected-names は、release を
 * 呼ぶまで応答しない。応答の中身は要求を受けた時点の `server.rejectedNames` を
 * 写し取って持つ（取り外しの前に始まった取り直しの古い応答を再現する）。
 */
let holdRejectedGets = false;
const rejectedGetReleases: (() => void)[] = [];

/** holdConfirms を true にした間の POST /api/tags/{id}/confirm は、release() を呼ぶまで応答しない。 */
let holdConfirms = false;
const confirmReleases: (() => void)[] = [];

/** holdNextMutation を true にした次の POST・PATCH は、release() を呼ぶまで応答しない（B3）。 */
let holdNextMutation = false;
let release: (() => void) | null = null;

/**
 * holdSynonymDeletes を true にした間の DELETE /api/tags/{id}/synonyms は、
 * それぞれの release を呼ぶまで応答しない。並行する複数の解除を、任意の
 * 順で・同じタイミングで解決させて確かめる（N5・並行する解除）。
 */
let holdSynonymDeletes = false;
const synonymDeleteReleases: (() => void)[] = [];

/**
 * holdGetsFrom を GET /api/tags の何回目（1始まり）から止めるかに設定すると、
 * それ以降の GET は release を呼ぶまで応答しない。応答の中身は、要求を
 * 受けた時点の `server.tags` を写し取って持つ（release を呼んだ時点の
 * `server.tags` ではない。Devin の指摘4のテストで、追い越された古い取得の
 * 応答が「その要求を送った時点でのサーバーの状態」を持つようにするため）。
 */
let holdGetsFrom: number | null = null;
const heldGetReleases: (() => void)[] = [];

function install() {
  const fetchMock = vi.fn<typeof fetch>();
  fetchMock.mockImplementation((input, init) => {
    const url = new URL(String(input), "http://localhost");
    const path = url.pathname;
    const method = init?.method ?? "GET";

    if (path === "/api/tags" && method === "GET") {
      server.getCalls += 1;
      if (server.failAllGets || server.failNextGet) {
        server.failNextGet = false;
        return Promise.resolve(
          jsonResponse({ code: "internal", message: "failed" }, 500),
        );
      }
      const snapshot = [...server.tags];
      if (holdGetsFrom !== null && server.getCalls >= holdGetsFrom) {
        return new Promise((resolve) => {
          heldGetReleases.push(() => resolve(jsonResponse({ items: snapshot })));
        });
      }
      return Promise.resolve(jsonResponse({ items: snapshot }));
    }

    if (path === "/api/tags/rejected-names" && method === "GET") {
      if (server.failRejectedGets) {
        return Promise.resolve(
          jsonResponse({ code: "internal", message: "failed" }, 500),
        );
      }
      const snapshot = [...server.rejectedNames];
      if (holdRejectedGets) {
        return new Promise((resolve) => {
          rejectedGetReleases.push(() => resolve(jsonResponse({ items: snapshot })));
        });
      }
      return Promise.resolve(jsonResponse({ items: snapshot }));
    }

    if (method !== "GET" && server.nextError !== null) {
      const { status, body } = server.nextError;
      server.nextError = null;
      return Promise.resolve(jsonResponse(body, status));
    }

    function maybeHold(respond: () => Response): Promise<Response> {
      if (!holdNextMutation) return Promise.resolve(respond());
      holdNextMutation = false;
      return new Promise((resolve) => {
        release = () => resolve(respond());
      });
    }

    if (path === "/api/tags/rejected-names" && method === "DELETE") {
      const name = url.searchParams.get("name") ?? "";
      server.rejectedNames = server.rejectedNames.filter((item) => item !== name);
      return Promise.resolve(jsonResponse(null, 204));
    }

    const confirmMatch = /^\/api\/tags\/(\d+)\/confirm$/.exec(path);
    if (confirmMatch && method === "POST") {
      server.confirmCalls += 1;
      const found = server.tags.find((t) => t.id === Number(confirmMatch[1]));
      if (found === undefined) {
        return Promise.resolve(
          jsonResponse({ code: "tag_not_found", message: "Tag not found." }, 404),
        );
      }
      const respond = () => {
        found.tentative = false;
        return jsonResponse(found);
      };
      if (!holdConfirms) return Promise.resolve(respond());
      return new Promise((resolve) => {
        confirmReleases.push(() => resolve(respond()));
      });
    }

    const rejectMatch = /^\/api\/tags\/(\d+)\/reject$/.exec(path);
    if (rejectMatch && method === "POST") {
      const found = server.tags.find((t) => t.id === Number(rejectMatch[1]));
      if (found === undefined) {
        return Promise.resolve(
          jsonResponse({ code: "tag_not_found", message: "Tag not found." }, 404),
        );
      }
      if (!found.tentative) {
        return Promise.resolve(
          jsonResponse(
            { code: "tag_not_tentative", message: "Tag is not tentative." },
            409,
          ),
        );
      }
      server.tags = server.tags.filter((t) => t.id !== found.id);
      server.rejectedNames = [...server.rejectedNames, found.name].sort();
      return Promise.resolve(jsonResponse(null, 204));
    }

    if (path === "/api/tags" && method === "POST") {
      if (server.failNextCreate) {
        server.failNextCreate = false;
        return Promise.resolve(
          jsonResponse({ code: "internal", message: "作成できませんでした" }, 500),
        );
      }
      const body = JSON.parse(String(init?.body)) as { name: string };
      const conflict = server.tags.find(
        (t) => t.name === body.name || t.synonyms.includes(body.name),
      );
      if (conflict !== undefined) {
        return maybeHold(() => takenResponse(body.name, conflict.name));
      }
      const created = tag({ id: server.nextId, name: body.name });
      server.nextId += 1;
      return maybeHold(() => {
        server.tags.push(created);
        server.rejectedNames = server.rejectedNames.filter((n) => n !== body.name);
        return jsonResponse(created, 201);
      });
    }

    const patchMatch = /^\/api\/tags\/(\d+)$/.exec(path);
    if (patchMatch && method === "PATCH") {
      const id = Number(patchMatch[1]);
      const found = server.tags.find((t) => t.id === id);
      if (found === undefined) {
        return Promise.resolve(
          jsonResponse({ code: "tag_not_found", message: "Tag not found." }, 404),
        );
      }
      if (server.failNextPatch) {
        server.failNextPatch = false;
        return Promise.resolve(
          jsonResponse({ code: "internal", message: "Could not rename." }, 500),
        );
      }
      const body = JSON.parse(String(init?.body)) as { name: string };
      const conflict = server.tags.find(
        (t) => t.id !== id && (t.name === body.name || t.synonyms.includes(body.name)),
      );
      if (conflict !== undefined) {
        return maybeHold(() => takenResponse(body.name, conflict.name));
      }
      return maybeHold(() => {
        if (found.name !== body.name) found.tentative = false;
        found.name = body.name;
        server.rejectedNames = server.rejectedNames.filter((n) => n !== body.name);
        return jsonResponse(found);
      });
    }

    if (patchMatch && method === "DELETE") {
      const id = Number(patchMatch[1]);
      const index = server.tags.findIndex((t) => t.id === id);
      if (index === -1) {
        return Promise.resolve(
          jsonResponse({ code: "tag_not_found", message: "Tag not found." }, 404),
        );
      }
      server.tags.splice(index, 1);
      return Promise.resolve(jsonResponse(null, 204));
    }

    const mergeMatch = /^\/api\/tags\/(\d+)\/merge$/.exec(path);
    if (mergeMatch && method === "POST") {
      const id = Number(mergeMatch[1]);
      const body = JSON.parse(String(init?.body)) as { sourceIds: number[] };
      const target = server.tags.find((t) => t.id === id);
      if (target === undefined) {
        return Promise.resolve(
          jsonResponse({ code: "tag_not_found", message: "Tag not found." }, 404),
        );
      }
      if (server.failNextMerge) {
        server.failNextMerge = false;
        return maybeHold(() =>
          jsonResponse({ code: "internal", message: "Could not merge." }, 500),
        );
      }
      // 無い統合元は飛ばして notFoundIds に載せる。統合元がすべて無ければ統合先は
      // 変わらない（specs/036-tag-admin-scale/contracts/screen-api.md §2）。
      return maybeHold(() => {
        const notFoundIds: number[] = [];
        for (const sourceId of body.sourceIds) {
          const source = server.tags.find((t) => t.id === sourceId);
          if (source === undefined) {
            notFoundIds.push(sourceId);
            continue;
          }
          target.synonyms = [...target.synonyms, source.name, ...source.synonyms];
          target.videoCount += source.videoCount;
          target.tentative = false;
          server.tags = server.tags.filter((t) => t.id !== source.id);
        }
        return jsonResponse({ tag: target, notFoundIds });
      });
    }

    const synonymsMatch = /^\/api\/tags\/(\d+)\/synonyms$/.exec(path);
    if (synonymsMatch && method === "POST") {
      const id = Number(synonymsMatch[1]);
      const target = server.tags.find((t) => t.id === id);
      if (target === undefined) {
        return Promise.resolve(
          jsonResponse({ code: "tag_not_found", message: "Tag not found." }, 404),
        );
      }
      if (server.failNextSynonymsPost) {
        server.failNextSynonymsPost = false;
        return maybeHold(() =>
          jsonResponse({ code: "internal", message: "Could not add." }, 500),
        );
      }
      if (server.forceMergeRequiredOnce) {
        server.forceMergeRequiredOnce = false;
        return maybeHold(() =>
          jsonResponse(
            {
              code: "tag_merge_required",
              tagName: (JSON.parse(String(init?.body)) as { name: string }).name,
              message: "server-side message",
            },
            409,
          ),
        );
      }
      const body = JSON.parse(String(init?.body)) as {
        name: string;
        mergeTagId?: number;
      };
      if (target.synonyms.includes(body.name)) {
        // 既にこのタグのシノニムである名前の登録は、何も変えずに 200 を返す。
        return maybeHold(() => jsonResponse(target));
      }
      if (target.name === body.name) {
        return maybeHold(() => takenResponse(body.name, target.name));
      }
      const ownedBy = server.tags.find(
        (t) => t.id !== id && (t.name === body.name || t.synonyms.includes(body.name)),
      );
      if (ownedBy !== undefined) {
        if (ownedBy.name === body.name) {
          // 別のタグの元の名前。承諾（mergeTagId が一致）していなければ
          // tag_merge_required。
          if (body.mergeTagId !== ownedBy.id) {
            return maybeHold(() =>
              jsonResponse(
                {
                  code: "tag_merge_required",
                  tagName: ownedBy.name,
                  message: "server-side message",
                },
                409,
              ),
            );
          }
          return maybeHold(() => {
            target.synonyms = [...target.synonyms, ownedBy.name, ...ownedBy.synonyms];
            server.tags = server.tags.filter((t) => t.id !== ownedBy.id);
            target.synonyms = [...target.synonyms, body.name].filter(
              (name, index, all) => all.indexOf(name) === index,
            );
            return jsonResponse(target);
          });
        }
        // 別のタグの既存のシノニムとの衝突。
        return maybeHold(() => takenResponse(body.name, ownedBy.name));
      }
      return maybeHold(() => {
        target.synonyms = [...target.synonyms, body.name];
        target.tentative = false;
        server.rejectedNames = server.rejectedNames.filter((n) => n !== body.name);
        return jsonResponse(target);
      });
    }

    if (synonymsMatch && method === "DELETE") {
      const id = Number(synonymsMatch[1]);
      const target = server.tags.find((t) => t.id === id);
      if (target === undefined) {
        return Promise.resolve(
          jsonResponse({ code: "tag_not_found", message: "Tag not found." }, 404),
        );
      }
      const name = url.searchParams.get("name") ?? "";
      const respond = () => {
        target.synonyms = target.synonyms.filter((s) => s !== name);
        return jsonResponse(null, 204);
      };
      if (!holdSynonymDeletes) return Promise.resolve(respond());
      return new Promise((resolve) => {
        synonymDeleteReleases.push(() => resolve(respond()));
      });
    }

    throw new Error(`unexpected request: ${method} ${path}`);
  });
  vi.stubGlobal("fetch", fetchMock);
  return fetchMock;
}

function renderPage() {
  return render(
    <MemoryRouter>
      <TooltipProvider>
        <ToastProvider>
          <TagsPage />
        </ToastProvider>
      </TooltipProvider>
    </MemoryRouter>,
  );
}

beforeEach(() => {
  __resetTagsForTest();
  server.tags = [
    tag({ id: 1, name: "旅行" }),
    tag({ id: 2, name: "Anime", synonyms: ["アニメ"], videoCount: 3 }),
    tag({ id: 3, name: "Drama" }),
  ];
  server.nextId = 100;
  server.getCalls = 0;
  server.failNextGet = false;
  server.failAllGets = false;
  server.failNextCreate = false;
  server.failNextPatch = false;
  server.failNextMerge = false;
  server.failNextSynonymsPost = false;
  server.forceMergeRequiredOnce = false;
  server.nextError = null;
  holdNextMutation = false;
  release = null;
  holdSynonymDeletes = false;
  synonymDeleteReleases.length = 0;
  holdGetsFrom = null;
  heldGetReleases.length = 0;
  server.rejectedNames = [];
  server.failRejectedGets = false;
  server.confirmCalls = 0;
  holdConfirms = false;
  confirmReleases.length = 0;
  holdRejectedGets = false;
  rejectedGetReleases.length = 0;
  // 一覧の仮想化（useWindowVirtualizer）は文書をスクロールする。jsdom には
  // window.scrollTo が無いので、何もしない実装に置き換える。
  vi.spyOn(window, "scrollTo").mockImplementation(() => {});
});

afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe("TagsPage", () => {
  it("一覧を本数つきで出し、件数の行に総数が出る", async () => {
    install();
    renderPage();

    expect(await screen.findByTitle("旅行")).toBeDefined();
    expect(screen.getByTitle("Anime")).toBeDefined();
    expect(screen.getByText("Synonyms: アニメ")).toBeDefined();
    expect(screen.getByText("3 tags")).toBeDefined();
    // 本数 0 のタグも出る。
    const dramaRow = screen.getByTitle("Drama").closest("div")!.parentElement!;
    expect(dramaRow.textContent).toContain("0 videos");
  });

  it("検索は名前とシノニムのどちらでも、大文字小文字を区別せず絞り込む（受け入れ条件18）", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("旅行");

    const search = screen.getByRole("searchbox", { name: "Search tags" });
    await user.type(search, "アニ");
    expect(await screen.findByText("1 of 3 tags")).toBeDefined();
    expect(screen.getByTitle("Anime")).toBeDefined();
    expect(screen.queryByTitle("旅行")).toBeNull();
    expect(screen.queryByTitle("Drama")).toBeNull();

    await user.clear(search);
    await user.type(search, "ani");
    expect(await screen.findByText("1 of 3 tags")).toBeDefined();
    expect(screen.getByTitle("Anime")).toBeDefined();

    await user.clear(search);
    expect(await screen.findByText("3 tags")).toBeDefined();
    expect(screen.getByTitle("旅行")).toBeDefined();
  });

  it("検索はライブラリと同じ照合形で、全角・半角やかなの違いを同じものとして照らす（受け入れ条件8）", async () => {
    const user = userEvent.setup();
    server.tags = [
      tag({ id: 1, name: "action", synonyms: [] }),
      tag({ id: 2, name: "Anime", synonyms: ["アニメ"] }),
      tag({ id: 3, name: "旅行" }),
    ];
    install();
    renderPage();
    await screen.findByTitle("旅行");

    const search = screen.getByRole("searchbox", { name: "Search tags" });
    await user.type(search, "ＡＣＴＩＯＮ");
    expect(await screen.findByText("1 of 3 tags")).toBeDefined();
    expect(screen.getByTitle("action")).toBeDefined();
    expect(screen.queryByTitle("Anime")).toBeNull();

    // シノニムも照合形で照らす（ひらがなで打ってもカタカナのシノニムに当たる）。
    await user.clear(search);
    await user.type(search, "あにめ");
    expect(await screen.findByText("1 of 3 tags")).toBeDefined();
    expect(screen.getByTitle("Anime")).toBeDefined();
    expect(screen.queryByTitle("action")).toBeNull();

    await user.clear(search);
    await user.type(search, "ｱｸｼｮﾝ");
    expect(await screen.findByText('No tags match "ｱｸｼｮﾝ"')).toBeDefined();
  });

  it("検索で一致が無いときは、タグが無い状態と別の表示になり、そこから入力を消せる", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("旅行");

    const search = screen.getByRole("searchbox", { name: "Search tags" });
    await user.type(search, "存在しない語");

    expect(await screen.findByText('No tags match "存在しない語"')).toBeDefined();
    expect(screen.queryByText("No tags yet")).toBeNull();

    await user.click(screen.getByRole("button", { name: "Show all tags" }));
    expect(document.activeElement).toBe(search);
    expect(await screen.findByTitle("旅行")).toBeDefined();
  });

  it("タグが1つも無いときは空の状態を出し、検索の入力はdisabledのまま", async () => {
    server.tags = [];
    install();
    renderPage();

    expect(await screen.findByText("No tags yet")).toBeDefined();
    expect(
      screen.getByRole("searchbox", { name: "Search tags" }).hasAttribute("disabled"),
    ).toBe(true);
  });

  it("新しいタグを作ると本数0で名前順の位置に入り、名前へフォーカスが移る（受け入れ条件9）", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("旅行");

    await user.click(screen.getByRole("button", { name: "New tag" }));
    const input = screen.getByRole("textbox", { name: "New tag name" });
    expect(document.activeElement).toBe(input);
    await user.type(input, "Banana");
    await user.keyboard("{Enter}");

    const created = await screen.findByRole("link", {
      name: "Open the library filtered by Banana",
    });
    await waitFor(() => expect(document.activeElement).toBe(created));
    expect(screen.getByText("4 tags")).toBeDefined();
  });

  // Devin の指摘4: 開いたときにすでに共有の一覧を持っていると（別の画面から
  // 移ってきた等）、画面が開くたびの取り直し（reload）が作成の前に始まり、
  // 追い越されたまま作成の後に届くことがある。その追い越された取得の
  // 戻り値をそのまま画面へ反映すると、確定した作成をその場で消してしまう。
  // `refreshTags` の generation ガードは「held を実際に更新した、最新の
  // 取得」だけを `subscribeTags` へ通知するので、そちらだけを信頼していれば
  // 消えない（TagsPage.tsx の `reload`）。
  it("追い越された取り直しの戻り値で、確定済みの作成を消さない（Devinの指摘4）", async () => {
    const user = userEvent.setup();
    install();
    // 別の画面ですでに一覧を取得済み（held あり）のまま、この画面を開く
    // （マウント時の1回目の GET を数えておく）。
    await refreshTags();
    expect(server.getCalls).toBe(1);

    // マウントの reload（2回目の GET）を止める。
    holdGetsFrom = 2;
    renderPage();
    await screen.findByTitle("旅行");
    await waitFor(() => expect(heldGetReleases).toHaveLength(1));

    // 作成の POST は止めない。成功後の afterTagCreated の取り直し（3回目の
    // GET）も同じく止め、まだ新しいタグを含まない2回目の応答が
    // 追い越されたまま先に届く状況を作る。
    await user.click(screen.getByRole("button", { name: "New tag" }));
    const input = screen.getByRole("textbox", { name: "New tag name" });
    await user.type(input, "新規タグ");
    await user.keyboard("{Enter}");

    const created = await screen.findByRole("link", {
      name: "Open the library filtered by 新規タグ",
    });
    await waitFor(() => expect(heldGetReleases).toHaveLength(2));

    // 追い越された2回目（作成前のスナップショット）を、3回目より先に解決する。
    heldGetReleases[0]!();
    await new Promise((resolve) => setTimeout(resolve, 0));
    expect(created.isConnected).toBe(true);
    expect(screen.getByRole("link", { name: "Open the library filtered by 新規タグ" }));

    // 3回目（作成を含む、最新の取得）が届いても、引き続き出ている。
    heldGetReleases[1]!();
    await waitFor(() =>
      expect(
        screen.getByRole("link", { name: "Open the library filtered by 新規タグ" }),
      ).toBeDefined(),
    );
  });

  it("既存の名前と重なる作成は理由を出し、入力を残す", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("旅行");

    await user.click(screen.getByRole("button", { name: "New tag" }));
    const input = screen.getByRole("textbox", { name: "New tag name" });
    await user.type(input, "旅行");
    await user.keyboard("{Enter}");

    expect(await screen.findByText('A tag named "旅行" already exists.')).toBeDefined();
    expect((input as HTMLInputElement).value).toBe("旅行");
  });

  it("改名すると新しい名前で表示され、既存名との重なりは理由を出す（受け入れ条件10）", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("旅行");

    const row = screen.getByTitle("旅行").closest("div")!.parentElement!;
    await user.click(within(row).getByRole("button", { name: "Rename" }));

    const input = await screen.findByRole("textbox", { name: 'New name for "旅行"' });
    expect(document.activeElement).toBe(input);

    // 既存のシノニムと重なる改名は拒否され、理由が画面に出る。
    await user.clear(input);
    await user.type(input, "アニメ");
    await user.keyboard("{Enter}");
    expect(
      await screen.findByText('"アニメ" is already a synonym of the tag "Anime".'),
    ).toBeDefined();

    await user.clear(input);
    await user.type(input, "旅行2024");
    await user.keyboard("{Enter}");

    expect(await screen.findByTitle("旅行2024")).toBeDefined();
    expect(screen.queryByTitle("旅行")).toBeNull();
  });

  it("改名中にEscを押すと、何も変えずに戻り、フォーカスが「改名」へ戻る（N3）", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("旅行");

    const row = screen.getByTitle("旅行").closest("div")!.parentElement!;
    const renameButton = within(row).getByRole("button", { name: "Rename" });
    await user.click(renameButton);
    const input = await screen.findByRole("textbox", { name: 'New name for "旅行"' });
    await user.clear(input);
    await user.type(input, "捨てる名前");
    await user.keyboard("{Escape}");

    expect(await screen.findByTitle("旅行")).toBeDefined();
    expect(screen.queryByTitle("捨てる名前")).toBeNull();
    await waitFor(() => expect(document.activeElement).toBe(renameButton));
  });

  it("作成中にEscを押すと、何も作らずフォーカスが「新しいタグ」へ戻る（N3）", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("旅行");

    const createButton = screen.getByRole("button", { name: "New tag" });
    await user.click(createButton);
    const input = screen.getByRole("textbox", { name: "New tag name" });
    await user.type(input, "捨てる名前");
    await user.keyboard("{Escape}");

    expect(screen.queryByRole("textbox", { name: "New tag name" })).toBeNull();
    expect(document.activeElement).toBe(createButton);
  });

  it("別のタブで消したタグを改名しようとすると、もう無いことが伝わり一覧が取り直される", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("旅行");

    const row = screen.getByTitle("旅行").closest("div")!.parentElement!;
    await user.click(within(row).getByRole("button", { name: "Rename" }));
    const input = await screen.findByRole("textbox", { name: 'New name for "旅行"' });

    // 改名を試みる前に、別のタブで削除されたことにする。
    server.tags = server.tags.filter((t) => t.name !== "旅行");

    await user.clear(input);
    await user.type(input, "旅行2024");
    await user.keyboard("{Enter}");

    expect(
      await screen.findByText("This tag no longer exists, so the list was reloaded"),
    ).toBeDefined();
    expect(screen.queryByTitle("旅行")).toBeNull();
    expect(screen.getByText("2 tags")).toBeDefined();
  });

  it("削除の確認は外れる本数を示し、確定すると消えて次の行の改名へフォーカスが移る（受け入れ条件11）", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("Anime");

    const row = screen.getByTitle("Anime").closest("div")!.parentElement!;
    await user.click(within(row).getByRole("button", { name: "More actions" }));
    await user.click(await screen.findByRole("menuitem", { name: "Delete…" }));

    const dialog = await screen.findByRole("dialog", { name: 'Delete "Anime"' });
    expect(
      within(dialog).getByText(
        "This tag will be removed from 3 videos. This can't be undone.",
      ),
    ).toBeDefined();

    await user.click(within(dialog).getByRole("button", { name: "Delete" }));

    await waitFor(() => expect(screen.queryByTitle("Anime")).toBeNull());
    expect(await screen.findByText('Deleted "Anime"')).toBeDefined();
    // Anime の次は Drama（自然順で 旅行 < Anime < Drama）。
    await waitFor(() =>
      expect(document.activeElement).toBe(
        within(screen.getByTitle("Drama").closest("div")!.parentElement!).getByRole(
          "button",
          { name: "Rename" },
        ),
      ),
    );
  });

  it("0本のタグの削除確認は「どの動画にも付いていません」と出す", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("Drama");

    const row = screen.getByTitle("Drama").closest("div")!.parentElement!;
    await user.click(within(row).getByRole("button", { name: "More actions" }));
    await user.click(await screen.findByRole("menuitem", { name: "Delete…" }));

    const dialog = await screen.findByRole("dialog", { name: 'Delete "Drama"' });
    expect(within(dialog).getByText("This tag isn't on any videos.")).toBeDefined();
  });

  it("削除の確認でEscを押すと何も変わらず、フォーカスがその行の「その他の操作」へ戻る（B2）", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("Drama");

    const row = screen.getByTitle("Drama").closest("div")!.parentElement!;
    const menuButton = within(row).getByRole("button", { name: "More actions" });
    await user.click(menuButton);
    await user.click(await screen.findByRole("menuitem", { name: "Delete…" }));
    await screen.findByRole("dialog", { name: 'Delete "Drama"' });

    await user.keyboard("{Escape}");

    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(screen.getByTitle("Drama")).toBeDefined();
    // その他の操作のメニュー項目（削除…）はメニューが閉じるともう無いので、
    // ModalFrame の「前のフォーカスへ戻す」だけには頼らず、その行の
    // 「その他の操作」へ明示的に戻す。
    await waitFor(() => expect(document.activeElement).toBe(menuButton));
  });

  it("削除の確認の「キャンセル」を押しても、フォーカスがその行の「その他の操作」へ戻る（B2）", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("Drama");

    const row = screen.getByTitle("Drama").closest("div")!.parentElement!;
    const menuButton = within(row).getByRole("button", { name: "More actions" });
    await user.click(menuButton);
    await user.click(await screen.findByRole("menuitem", { name: "Delete…" }));
    const dialog = await screen.findByRole("dialog", { name: 'Delete "Drama"' });

    await user.click(within(dialog).getByRole("button", { name: "Cancel" }));

    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    await waitFor(() => expect(document.activeElement).toBe(menuButton));
  });

  it("別のタブで消したタグの削除を試みると、もう無いことが伝わり次の行の改名へフォーカスが移る（B2）", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("Anime");

    const row = screen.getByTitle("Anime").closest("div")!.parentElement!;
    await user.click(within(row).getByRole("button", { name: "More actions" }));
    await user.click(await screen.findByRole("menuitem", { name: "Delete…" }));
    const dialog = await screen.findByRole("dialog", { name: 'Delete "Anime"' });

    // 確認を開いたあと、別のタブで先に削除されたことにする。
    server.tags = server.tags.filter((t) => t.name !== "Anime");

    await user.click(within(dialog).getByRole("button", { name: "Delete" }));

    expect(
      await screen.findByText("This tag no longer exists, so the list was reloaded"),
    ).toBeDefined();
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    // Anime の次は Drama（自然順で 旅行 < Anime < Drama）。
    await waitFor(() =>
      expect(document.activeElement).toBe(
        within(screen.getByTitle("Drama").closest("div")!.parentElement!).getByRole(
          "button",
          { name: "Rename" },
        ),
      ),
    );
  });

  it("キーボードだけで検索の入力に届く（本文で最初のTab）", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("旅行");

    await user.tab();
    expect(document.activeElement).toBe(
      screen.getByRole("searchbox", { name: "Search tags" }),
    );
  });

  it("読み込みに失敗すると再試行でき、成功すると一覧が出る", async () => {
    const fetchMock = vi.fn<typeof fetch>();
    let first = true;
    fetchMock.mockImplementation((input) => {
      const url = new URL(String(input), "http://localhost");
      if (url.pathname === "/api/tags" && first) {
        first = false;
        return Promise.resolve(
          jsonResponse({ code: "internal", message: "failed" }, 500),
        );
      }
      return Promise.resolve(jsonResponse({ items: server.tags }));
    });
    vi.stubGlobal("fetch", fetchMock);

    renderPage();
    expect(await screen.findByText("Couldn't load the tags")).toBeDefined();

    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: "Retry" }));
    expect(await screen.findByTitle("旅行")).toBeDefined();
  });

  it("IME変換中のEnterとEscは、作成・改名・検索のどの入力でも無視される（B1）", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("旅行");

    // 作成の入力。
    await user.click(screen.getByRole("button", { name: "New tag" }));
    const createInput = screen.getByRole("textbox", { name: "New tag name" });
    await user.type(createInput, "IME作成中");
    fireEvent.keyDown(createInput, { key: "Enter", keyCode: 229 });
    expect(screen.queryByTitle("IME作成中")).toBeNull();
    fireEvent.keyDown(createInput, { key: "Escape", keyCode: 229 });
    expect(screen.getByRole("textbox", { name: "New tag name" })).toBeDefined();
    fireEvent.keyDown(createInput, { key: "Enter" });
    expect(await screen.findByTitle("IME作成中")).toBeDefined();

    // 改名の入力。
    const row = screen.getByTitle("旅行").closest("div")!.parentElement!;
    await user.click(within(row).getByRole("button", { name: "Rename" }));
    const renameInput = await screen.findByRole("textbox", {
      name: 'New name for "旅行"',
    });
    await user.clear(renameInput);
    await user.type(renameInput, "旅行2024");
    fireEvent.keyDown(renameInput, { key: "Enter", keyCode: 229 });
    expect(screen.queryByTitle("旅行2024")).toBeNull();
    fireEvent.keyDown(renameInput, { key: "Escape", keyCode: 229 });
    expect(screen.getByRole("textbox", { name: 'New name for "旅行"' })).toBeDefined();
    fireEvent.keyDown(renameInput, { key: "Enter" });
    expect(await screen.findByTitle("旅行2024")).toBeDefined();

    // 検索の入力。
    const search = screen.getByRole("searchbox", { name: "Search tags" });
    await user.type(search, "存在しない語");
    fireEvent.keyDown(search, { key: "Escape", keyCode: 229 });
    expect((search as HTMLInputElement).value).toBe("存在しない語");
    fireEvent.keyDown(search, { key: "Escape" });
    expect((search as HTMLInputElement).value).toBe("");
  });

  it("作成中はaria-busyだけを付けて入力をdisabledにせず、tag_name_taken後もフォーカスと値が残る（B3）", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("旅行");

    await user.click(screen.getByRole("button", { name: "New tag" }));
    const input = screen.getByRole("textbox", { name: "New tag name" });
    await user.type(input, "旅行");

    holdNextMutation = true;
    await user.keyboard("{Enter}");

    await waitFor(() => expect(input.getAttribute("aria-busy")).toBe("true"));
    expect(input.hasAttribute("disabled")).toBe(false);
    expect(document.activeElement).toBe(input);

    release?.();

    expect(await screen.findByText('A tag named "旅行" already exists.')).toBeDefined();
    expect(document.activeElement).toBe(input);
    expect((input as HTMLInputElement).value).toBe("旅行");
    await waitFor(() => expect(input.getAttribute("aria-busy")).toBeNull());
  });

  it("改名中はaria-busyだけを付けて入力をdisabledにせず、tag_name_taken後もフォーカスと値が残る（B3）", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("旅行");

    const row = screen.getByTitle("旅行").closest("div")!.parentElement!;
    await user.click(within(row).getByRole("button", { name: "Rename" }));
    const input = await screen.findByRole("textbox", { name: 'New name for "旅行"' });
    await user.clear(input);
    await user.type(input, "アニメ");

    holdNextMutation = true;
    await user.keyboard("{Enter}");

    await waitFor(() => expect(input.getAttribute("aria-busy")).toBe("true"));
    expect(input.hasAttribute("disabled")).toBe(false);
    expect(document.activeElement).toBe(input);

    release?.();

    expect(
      await screen.findByText('"アニメ" is already a synonym of the tag "Anime".'),
    ).toBeDefined();
    expect(document.activeElement).toBe(input);
    expect((input as HTMLInputElement).value).toBe("アニメ");
    await waitFor(() => expect(input.getAttribute("aria-busy")).toBeNull());
  });

  it("改名中も本数・改名・その他の操作は出したままで、同じ行の要素のままになる（B4）", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("Anime");

    const row = screen.getByTitle("Anime").closest("div")!.parentElement!;
    await user.click(within(row).getByRole("button", { name: "Rename" }));

    const input = await screen.findByRole("textbox", { name: 'New name for "Anime"' });
    expect(input.closest("[data-tag-id]")).toBe(row);
    expect(row.textContent).toContain("3 videos");
    expect(within(row).getByRole("button", { name: "Rename" })).toBeDefined();
    expect(within(row).getByRole("button", { name: "More actions" })).toBeDefined();
  });

  it("tag_name_taken以外の改名の失敗はtext-sm・role=alertで、tag_name_takenはrole=alert無しで出す（N2）", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("旅行");

    const row = screen.getByTitle("旅行").closest("div")!.parentElement!;
    await user.click(within(row).getByRole("button", { name: "Rename" }));
    const input = await screen.findByRole("textbox", { name: 'New name for "旅行"' });

    server.failNextPatch = true;
    await user.clear(input);
    await user.type(input, "旅行2024");
    await user.keyboard("{Enter}");

    const alert = await screen.findByRole("alert");
    expect(alert.textContent).toBe("Something went wrong on the server.");
    expect(alert.className).toContain("text-sm");

    await user.clear(input);
    await user.type(input, "アニメ");
    await user.keyboard("{Enter}");

    const taken = await screen.findByText(
      '"アニメ" is already a synonym of the tag "Anime".',
    );
    expect(taken.getAttribute("role")).toBeNull();
    expect(taken.className).toContain("text-xs");
    expect(input.getAttribute("aria-invalid")).toBe("true");
  });

  it("通信失敗は作成・改名・シノニム追加の名前入力を無効扱いせず、alertで伝える", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("旅行");

    await user.click(screen.getByRole("button", { name: "New tag" }));
    const createInput = screen.getByRole("textbox", { name: "New tag name" });
    await user.type(createInput, "未登録");
    server.failNextCreate = true;
    await user.keyboard("{Enter}");
    expect((await screen.findByRole("alert")).textContent).toBe(
      "Something went wrong on the server.",
    );
    expect(createInput.getAttribute("aria-invalid")).toBeNull();
    await user.click(screen.getByRole("button", { name: "Cancel" }));

    const row = screen.getByTitle("旅行").closest("div")!.parentElement!;
    await user.click(within(row).getByRole("button", { name: "Rename" }));
    const renameInput = screen.getByRole("textbox", { name: 'New name for "旅行"' });
    await user.clear(renameInput);
    await user.type(renameInput, "旅行2024");
    server.failNextPatch = true;
    await user.keyboard("{Enter}");
    expect((await screen.findByRole("alert")).textContent).toBe(
      "Something went wrong on the server.",
    );
    expect(renameInput.getAttribute("aria-invalid")).toBeNull();
    await user.keyboard("{Escape}");

    const dramaRow = screen.getByTitle("Drama").closest("div")!.parentElement!;
    await user.click(within(dramaRow).getByRole("button", { name: "Synonyms" }));
    const dialog = await screen.findByRole("dialog", { name: 'Synonyms of "Drama"' });
    const synonymInput = within(dialog).getByRole("textbox", { name: "Add synonym" });
    await user.type(synonymInput, "別名");
    server.failNextSynonymsPost = true;
    await user.keyboard("{Enter}");
    expect((await within(dialog).findByRole("alert")).textContent).toBe(
      "Something went wrong on the server.",
    );
    expect(synonymInput.getAttribute("aria-invalid")).toBeNull();
  });

  it("空白だけの検索は絞り込み中として数えない（N5）", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("旅行");

    const search = screen.getByRole("searchbox", { name: "Search tags" });
    await user.type(search, "   ");

    expect(screen.getByText("3 tags")).toBeDefined();
    expect(screen.queryByText(/ of 3 tags/)).toBeNull();
  });

  it("作成の直後に一覧を2回取り直さない（N6）", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("旅行");

    expect(server.getCalls).toBe(1);

    await user.click(screen.getByRole("button", { name: "New tag" }));
    const input = screen.getByRole("textbox", { name: "New tag name" });
    await user.type(input, "Banana");
    await user.keyboard("{Enter}");
    await screen.findByTitle("Banana");

    // api/tags.ts の afterTagCreated によるバックグラウンドの1回だけが増える
    // （TagsPage 自身は、その結果を待たずに作った1件をその場で重ねるので、
    // もう1回 GET /api/tags を送らない）。
    await waitFor(() => expect(server.getCalls).toBe(2));
    await new Promise((resolve) => setTimeout(resolve, 50));
    expect(server.getCalls).toBe(2);
  });

  it("tag_not_foundの取り直しに失敗しても、一覧を空白にせず今の一覧を残す（N6）", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("旅行");

    const row = screen.getByTitle("旅行").closest("div")!.parentElement!;
    await user.click(within(row).getByRole("button", { name: "Rename" }));
    const input = await screen.findByRole("textbox", { name: 'New name for "旅行"' });

    server.tags = server.tags.filter((t) => t.name !== "旅行");
    server.failNextGet = true;

    await user.clear(input);
    await user.type(input, "旅行2024");
    await user.keyboard("{Enter}");

    expect(
      await screen.findByText("This tag no longer exists, so the list was reloaded"),
    ).toBeDefined();
    expect(screen.getByTitle("Anime")).toBeDefined();
    expect(screen.getByTitle("Drama")).toBeDefined();
    expect(screen.queryByText("Couldn't load the tags")).toBeNull();
  });

  it("改名中に検索でその行が一致しなくなっても行は消えず、打っている途中の名前が残る（Devinの指摘1）", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("旅行");

    const row = screen.getByTitle("旅行").closest("div")!.parentElement!;
    await user.click(within(row).getByRole("button", { name: "Rename" }));
    const input = await screen.findByRole("textbox", { name: 'New name for "旅行"' });
    await user.clear(input);
    await user.type(input, "捨てない下書き");

    // 「旅行」はもう検索に一致しない条件へ変える。
    const search = screen.getByRole("searchbox", { name: "Search tags" });
    await user.type(search, "Anime");

    // 行は消えず、打っている途中の名前もそのまま残る。
    const stillInput = screen.getByRole("textbox", { name: 'New name for "旅行"' });
    expect(stillInput).toBe(input);
    expect((stillInput as HTMLInputElement).value).toBe("捨てない下書き");
    // 件数の行は実際の一致件数（Anime の1件）のままで、ピン留めした分は数えない。
    expect(screen.getByText("1 of 3 tags")).toBeDefined();
  });

  it("改名の送信中はEscで閉じない。閉じたあとに応答が届いて状態を書き換えることも無い（Devinの指摘2）", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("旅行");

    const row = screen.getByTitle("旅行").closest("div")!.parentElement!;
    await user.click(within(row).getByRole("button", { name: "Rename" }));
    const input = await screen.findByRole("textbox", { name: 'New name for "旅行"' });
    await user.clear(input);
    await user.type(input, "旅行2024");

    holdNextMutation = true;
    await user.keyboard("{Enter}");
    await waitFor(() => expect(input.getAttribute("aria-busy")).toBe("true"));

    // 送信中の Esc は無視される。
    await user.keyboard("{Escape}");
    expect(screen.getByRole("textbox", { name: 'New name for "旅行"' })).toBeDefined();

    release?.();
    expect(await screen.findByTitle("旅行2024")).toBeDefined();
  });

  it("作成の送信中はEscで閉じず、キャンセルのボタンもdisabledのまま（Devinの指摘2）", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("旅行");

    await user.click(screen.getByRole("button", { name: "New tag" }));
    const input = screen.getByRole("textbox", { name: "New tag name" });
    await user.type(input, "Banana");

    holdNextMutation = true;
    await user.keyboard("{Enter}");
    await waitFor(() => expect(input.getAttribute("aria-busy")).toBe("true"));

    expect(screen.getByRole("button", { name: "Cancel" }).hasAttribute("disabled")).toBe(
      true,
    );
    await user.keyboard("{Escape}");
    expect(screen.getByRole("textbox", { name: "New tag name" })).toBeDefined();

    release?.();
    expect(await screen.findByTitle("Banana")).toBeDefined();
  });

  it("改名の送信中はほかの行の改名も「新しいタグ」も始められない。応答が届けば再び始められる（Devinの指摘2）", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("旅行");

    const row = screen.getByTitle("旅行").closest("div")!.parentElement!;
    await user.click(within(row).getByRole("button", { name: "Rename" }));
    const input = await screen.findByRole("textbox", { name: 'New name for "旅行"' });
    await user.clear(input);
    await user.type(input, "旅行2024");

    holdNextMutation = true;
    await user.keyboard("{Enter}");
    await waitFor(() => expect(input.getAttribute("aria-busy")).toBe("true"));

    const animeRow = screen.getByTitle("Anime").closest("div")!.parentElement!;
    const animeRenameButton = within(animeRow).getByRole("button", { name: "Rename" });
    expect(animeRenameButton.hasAttribute("disabled")).toBe(true);
    await user.click(animeRenameButton);
    expect(screen.queryByRole("textbox", { name: 'New name for "Anime"' })).toBeNull();

    expect(screen.getByRole("button", { name: "New tag" }).hasAttribute("disabled")).toBe(
      true,
    );

    release?.();
    // 「旅行」の改名が終わって初めて、Anime の改名を始められる。応答が
    // すでに終わった「旅行」の改名を巻き戻すことは無い。
    expect(await screen.findByTitle("旅行2024")).toBeDefined();
    await user.click(within(animeRow).getByRole("button", { name: "Rename" }));
    expect(
      await screen.findByRole("textbox", { name: 'New name for "Anime"' }),
    ).toBeDefined();
  });

  it("作成の失敗の表示は、入力を打ち直すと消える（Devinの指摘3）", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("旅行");

    await user.click(screen.getByRole("button", { name: "New tag" }));
    const input = screen.getByRole("textbox", { name: "New tag name" });
    await user.type(input, "旅行");
    await user.keyboard("{Enter}");

    expect(await screen.findByText('A tag named "旅行" already exists.')).toBeDefined();

    await user.type(input, "2024");
    expect(screen.queryByText('A tag named "旅行" already exists.')).toBeNull();
  });

  it("改名の失敗の表示は、入力を打ち直すと消える（Devinの指摘3）", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("旅行");

    const row = screen.getByTitle("旅行").closest("div")!.parentElement!;
    await user.click(within(row).getByRole("button", { name: "Rename" }));
    const input = await screen.findByRole("textbox", { name: 'New name for "旅行"' });
    await user.clear(input);
    await user.type(input, "アニメ");
    await user.keyboard("{Enter}");

    expect(
      await screen.findByText('"アニメ" is already a synonym of the tag "Anime".'),
    ).toBeDefined();

    await user.type(input, "2024");
    expect(
      screen.queryByText('"アニメ" is already a synonym of the tag "Anime".'),
    ).toBeNull();
  });
});

describe("TagsPage 統合", () => {
  it("「その他の操作」の先頭に「別のタグへ統合…」、区切り線を挟んで「削除…」が並ぶ（統合は作成・改名より目立たない）", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("旅行");

    const row = screen.getByTitle("旅行").closest("div")!.parentElement!;
    await user.click(within(row).getByRole("button", { name: "More actions" }));
    const menu = await screen.findByRole("menu");
    const items = within(menu).getAllByRole("menuitem");
    expect(items.map((item) => item.textContent)).toEqual([
      "Merge into another tag…",
      "Delete…",
    ]);
  });

  it("統合すると統合元が一覧から消え統合先のシノニムに並び、フォーカスが統合先の名前へ移る（受け入れ条件12）", async () => {
    const user = userEvent.setup();
    const fetchMock = install();
    server.tags = [
      tag({ id: 1, name: "旅行", videoCount: 5 }),
      tag({ id: 2, name: "Anime", synonyms: ["アニメ"], videoCount: 3 }),
    ];
    renderPage();
    await screen.findByTitle("旅行");

    const row = screen.getByTitle("旅行").closest("div")!.parentElement!;
    await user.click(within(row).getByRole("button", { name: "More actions" }));
    await user.click(
      await screen.findByRole("menuitem", { name: "Merge into another tag…" }),
    );
    const dialog = await screen.findByRole("dialog", { name: 'Merge "旅行"' });

    const combo = within(dialog).getByRole("combobox", { name: "Tag to merge into" });
    const mergeButton = within(dialog).getByRole("button", { name: "Merge" });
    expect(mergeButton.hasAttribute("disabled")).toBe(true);

    await user.type(combo, "Anime");
    await user.click(await within(dialog).findByRole("option", { name: /Anime/ }));

    expect(
      within(dialog).getByText(
        'The 5 videos tagged "旅行" get the tag "Anime". "旅行" and its synonyms become synonyms of "Anime", and "旅行" leaves the tag list. This can\'t be undone.',
      ),
    ).toBeDefined();
    expect(mergeButton.hasAttribute("disabled")).toBe(false);

    await user.click(mergeButton);

    expect(await screen.findByText('Merged "旅行" into "Anime"')).toBeDefined();
    // 1 件の統合も sourceIds で送る（specs/036-tag-admin-scale/contracts/screen-api.md §2）。
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/tags/2/merge",
      expect.objectContaining({
        method: "POST",
        body: JSON.stringify({ sourceIds: [1] }),
      }),
    );
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(screen.queryByTitle("旅行")).toBeNull();
    expect(screen.getByText("Synonyms: アニメ · 旅行")).toBeDefined();
    await waitFor(() =>
      expect(document.activeElement).toBe(
        screen.getByRole("link", { name: "Open the library filtered by Anime" }),
      ),
    );
  });

  it("統合先の候補は統合元を除き、作成の行を持たない", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("旅行");

    const row = screen.getByTitle("旅行").closest("div")!.parentElement!;
    await user.click(within(row).getByRole("button", { name: "More actions" }));
    await user.click(
      await screen.findByRole("menuitem", { name: "Merge into another tag…" }),
    );
    const dialog = await screen.findByRole("dialog", { name: 'Merge "旅行"' });
    const combo = within(dialog).getByRole("combobox", { name: "Tag to merge into" });
    await user.click(combo);

    expect(within(dialog).queryByRole("option", { name: /旅行/ })).toBeNull();
    await user.type(combo, "存在しない語");
    expect(within(dialog).queryByText(/^Create "/)).toBeNull();
  });

  it("統合の確認でEscを押すと何も変わらず、フォーカスがその行の「その他の操作」へ戻る", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("旅行");

    const row = screen.getByTitle("旅行").closest("div")!.parentElement!;
    const menuButton = within(row).getByRole("button", { name: "More actions" });
    await user.click(menuButton);
    await user.click(
      await screen.findByRole("menuitem", { name: "Merge into another tag…" }),
    );
    await screen.findByRole("dialog", { name: 'Merge "旅行"' });

    await user.keyboard("{Escape}");

    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(screen.getByTitle("旅行")).toBeDefined();
    await waitFor(() => expect(document.activeElement).toBe(menuButton));
  });

  it("統合先が別のタブで消えていると、もう無いことが伝わり一覧が取り直される", async () => {
    const user = userEvent.setup();
    install();
    server.tags = [tag({ id: 1, name: "旅行" }), tag({ id: 2, name: "Anime" })];
    renderPage();
    await screen.findByTitle("旅行");

    const row = screen.getByTitle("旅行").closest("div")!.parentElement!;
    await user.click(within(row).getByRole("button", { name: "More actions" }));
    await user.click(
      await screen.findByRole("menuitem", { name: "Merge into another tag…" }),
    );
    const dialog = await screen.findByRole("dialog", { name: 'Merge "旅行"' });
    const combo = within(dialog).getByRole("combobox", { name: "Tag to merge into" });
    await user.type(combo, "Anime");
    await user.click(await within(dialog).findByRole("option", { name: /Anime/ }));

    server.tags = server.tags.filter((t) => t.name !== "Anime");

    await user.click(within(dialog).getByRole("button", { name: "Merge" }));

    expect(
      await screen.findByText("This tag no longer exists, so the list was reloaded"),
    ).toBeDefined();
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  });

  it("統合元が別のタブで消えていると（notFoundIds）、もう無いことが伝わり一覧が取り直される", async () => {
    const user = userEvent.setup();
    install();
    server.tags = [tag({ id: 1, name: "旅行" }), tag({ id: 2, name: "Anime" })];
    renderPage();
    await screen.findByTitle("旅行");

    const row = screen.getByTitle("旅行").closest("div")!.parentElement!;
    await user.click(within(row).getByRole("button", { name: "More actions" }));
    await user.click(
      await screen.findByRole("menuitem", { name: "Merge into another tag…" }),
    );
    const dialog = await screen.findByRole("dialog", { name: 'Merge "旅行"' });
    const combo = within(dialog).getByRole("combobox", { name: "Tag to merge into" });
    await user.type(combo, "Anime");
    await user.click(await within(dialog).findByRole("option", { name: /Anime/ }));

    server.tags = server.tags.filter((t) => t.name !== "旅行");

    await user.click(within(dialog).getByRole("button", { name: "Merge" }));

    expect(
      await screen.findByText("This tag no longer exists, so the list was reloaded"),
    ).toBeDefined();
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    await waitFor(() => expect(screen.queryByTitle("旅行")).toBeNull());
    expect(screen.queryByText(/^Merged /)).toBeNull();
  });

  it("統合先の候補が開いているときのEscは一覧だけを閉じ、窓は閉じない（B1）", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("旅行");

    const row = screen.getByTitle("旅行").closest("div")!.parentElement!;
    await user.click(within(row).getByRole("button", { name: "More actions" }));
    await user.click(
      await screen.findByRole("menuitem", { name: "Merge into another tag…" }),
    );
    const dialog = await screen.findByRole("dialog", { name: 'Merge "旅行"' });

    const combo = within(dialog).getByRole("combobox", { name: "Tag to merge into" });
    await user.click(combo);
    await user.type(combo, "A");
    expect(combo.getAttribute("aria-expanded")).toBe("true");

    // 一覧が開いているときの1回目のEscは、一覧だけを閉じる。窓は開いたまま。
    await user.keyboard("{Escape}");
    expect(combo.getAttribute("aria-expanded")).toBe("false");
    expect(screen.getByRole("dialog", { name: 'Merge "旅行"' })).toBeDefined();

    // 一覧が閉じている状態での2回目のEscで、窓が閉じる。
    await user.keyboard("{Escape}");
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  });

  it("IME変換中のEscは窓を閉じない（B2）", async () => {
    install();
    renderPage();
    await screen.findByTitle("旅行");

    const user = userEvent.setup();
    const row = screen.getByTitle("旅行").closest("div")!.parentElement!;
    await user.click(within(row).getByRole("button", { name: "More actions" }));
    await user.click(
      await screen.findByRole("menuitem", { name: "Merge into another tag…" }),
    );
    const dialog = await screen.findByRole("dialog", { name: 'Merge "旅行"' });
    const cancelButton = within(dialog).getByRole("button", { name: "Cancel" });
    cancelButton.focus();

    fireEvent.keyDown(cancelButton, { key: "Escape", keyCode: 229 });
    expect(screen.getByRole("dialog", { name: 'Merge "旅行"' })).toBeDefined();

    fireEvent.keyDown(cancelButton, { key: "Escape" });
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  });

  it("統合の要求が届く間はEscや×で閉じない（N3）", async () => {
    const user = userEvent.setup();
    install();
    server.tags = [tag({ id: 1, name: "旅行" }), tag({ id: 2, name: "Anime" })];
    renderPage();
    await screen.findByTitle("旅行");

    const row = screen.getByTitle("旅行").closest("div")!.parentElement!;
    await user.click(within(row).getByRole("button", { name: "More actions" }));
    await user.click(
      await screen.findByRole("menuitem", { name: "Merge into another tag…" }),
    );
    const dialog = await screen.findByRole("dialog", { name: 'Merge "旅行"' });
    const combo = within(dialog).getByRole("combobox", { name: "Tag to merge into" });
    await user.type(combo, "Anime");
    await user.click(await within(dialog).findByRole("option", { name: /Anime/ }));

    holdNextMutation = true;
    await user.click(within(dialog).getByRole("button", { name: "Merge" }));
    await waitFor(() =>
      expect(
        within(dialog).getByRole("button", { name: "Merging…" }).hasAttribute("disabled"),
      ).toBe(true),
    );

    await user.keyboard("{Escape}");
    expect(screen.getByRole("dialog", { name: 'Merge "旅行"' })).toBeDefined();

    await user.click(within(dialog).getByRole("button", { name: "Close" }));
    expect(screen.getByRole("dialog", { name: 'Merge "旅行"' })).toBeDefined();

    release?.();
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  });

  it("統合が失敗すると、フォーカスが「統合する」へ戻る（N4）", async () => {
    const user = userEvent.setup();
    install();
    server.tags = [tag({ id: 1, name: "旅行" }), tag({ id: 2, name: "Anime" })];
    renderPage();
    await screen.findByTitle("旅行");

    const row = screen.getByTitle("旅行").closest("div")!.parentElement!;
    await user.click(within(row).getByRole("button", { name: "More actions" }));
    await user.click(
      await screen.findByRole("menuitem", { name: "Merge into another tag…" }),
    );
    const dialog = await screen.findByRole("dialog", { name: 'Merge "旅行"' });
    const combo = within(dialog).getByRole("combobox", { name: "Tag to merge into" });
    await user.type(combo, "Anime");
    await user.click(await within(dialog).findByRole("option", { name: /Anime/ }));

    server.failNextMerge = true;
    const mergeButton = within(dialog).getByRole("button", { name: "Merge" });
    await user.click(mergeButton);

    expect(await within(dialog).findByRole("alert")).toBeDefined();
    await waitFor(() => expect(document.activeElement).toBe(mergeButton));
  });
});

describe("TagsPage シノニム", () => {
  it("今のシノニムをChipに出し、×で確認なしにその場で解除できる", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("Anime");

    const row = screen.getByTitle("Anime").closest("div")!.parentElement!;
    await user.click(within(row).getByRole("button", { name: "Synonyms" }));
    const dialog = await screen.findByRole("dialog", { name: 'Synonyms of "Anime"' });

    expect(within(dialog).getByText("アニメ")).toBeDefined();
    await user.click(
      within(dialog).getByRole("button", { name: 'Remove the synonym "アニメ"' }),
    );

    await waitFor(() => expect(within(dialog).queryByText("アニメ")).toBeNull());
    await waitFor(() => expect(screen.queryByText("Synonyms: アニメ")).toBeNull());
  });

  it("シノニムを追加すると窓の中の一覧に並び、入力が空に戻る", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("Drama");

    const row = screen.getByTitle("Drama").closest("div")!.parentElement!;
    await user.click(within(row).getByRole("button", { name: "Synonyms" }));
    const dialog = await screen.findByRole("dialog", { name: 'Synonyms of "Drama"' });
    const input = within(dialog).getByRole("textbox", { name: "Add synonym" });
    await user.type(input, "ドラマ");
    await user.keyboard("{Enter}");

    expect(await within(dialog).findByText("ドラマ")).toBeDefined();
    expect((input as HTMLInputElement).value).toBe("");
  });

  it("別のタグのシノニムと同じ名前の登録で、そのタグの名前が画面に出る（シノニム名の衝突）", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("Drama");

    const row = screen.getByTitle("Drama").closest("div")!.parentElement!;
    await user.click(within(row).getByRole("button", { name: "Synonyms" }));
    const dialog = await screen.findByRole("dialog", { name: 'Synonyms of "Drama"' });
    const input = within(dialog).getByRole("textbox", { name: "Add synonym" });
    await user.type(input, "アニメ");
    await user.keyboard("{Enter}");

    expect(
      await within(dialog).findByText(
        '"アニメ" is already a synonym of the tag "Anime".',
      ),
    ).toBeDefined();
  });

  it("既存のタグの名前をシノニムとして登録しようとすると統合の確認が出て、承諾すると統合される（受け入れ条件17）", async () => {
    const user = userEvent.setup();
    install();
    server.tags = [
      tag({ id: 1, name: "Anime", videoCount: 2 }),
      tag({ id: 2, name: "anime", videoCount: 10 }),
    ];
    renderPage();
    await screen.findByTitle("Anime");

    const row = screen.getByTitle("Anime").closest("div")!.parentElement!;
    await user.click(within(row).getByRole("button", { name: "Synonyms" }));
    const dialog = await screen.findByRole("dialog", { name: 'Synonyms of "Anime"' });
    const input = within(dialog).getByRole("textbox", { name: "Add synonym" });
    await user.type(input, "anime");
    await user.keyboard("{Enter}");

    expect(
      await within(dialog).findByText(
        '"anime" is a tag on 10 videos. Merging it into "Anime" adds "Anime" to those videos, and "anime" becomes a synonym of "Anime". "anime" leaves the tag list.',
      ),
    ).toBeDefined();
    const backButton = within(dialog).getByRole("button", { name: "Back" });
    expect(document.activeElement).toBe(backButton);

    await user.click(within(dialog).getByRole("button", { name: "Merge" }));

    await waitFor(() => expect(within(dialog).queryByText(/is a tag on/)).toBeNull());
    expect(within(dialog).getByText("anime")).toBeDefined();
    expect(server.tags.some((t) => t.name === "anime")).toBe(false);
  });

  it("シノニムが無いタグとの統合の確認では「とそのシノニム」を省く", async () => {
    const user = userEvent.setup();
    install();
    server.tags = [
      tag({ id: 1, name: "Anime", videoCount: 2 }),
      tag({ id: 2, name: "anime", synonyms: ["アニメーション"], videoCount: 10 }),
    ];
    renderPage();
    await screen.findByTitle("Anime");

    const row = screen.getByTitle("Anime").closest("div")!.parentElement!;
    await user.click(within(row).getByRole("button", { name: "Synonyms" }));
    const dialog = await screen.findByRole("dialog", { name: 'Synonyms of "Anime"' });
    const input = within(dialog).getByRole("textbox", { name: "Add synonym" });
    await user.type(input, "anime");
    await user.keyboard("{Enter}");

    expect(
      await within(dialog).findByText(
        '"anime" is a tag on 10 videos. Merging it into "Anime" adds "Anime" to those videos, and "anime" and its synonyms become synonyms of "Anime". "anime" leaves the tag list.',
      ),
    ).toBeDefined();
  });

  it("シノニム登録に伴う統合の確認で「戻る」を押すと何も変えず入力へ戻り、文字が残る（取り消すと何も変わらない）", async () => {
    const user = userEvent.setup();
    install();
    server.tags = [
      tag({ id: 1, name: "Anime", videoCount: 2 }),
      tag({ id: 2, name: "anime", videoCount: 10 }),
    ];
    renderPage();
    await screen.findByTitle("Anime");

    const row = screen.getByTitle("Anime").closest("div")!.parentElement!;
    await user.click(within(row).getByRole("button", { name: "Synonyms" }));
    const dialog = await screen.findByRole("dialog", { name: 'Synonyms of "Anime"' });
    const input = within(dialog).getByRole("textbox", { name: "Add synonym" });
    await user.type(input, "anime");
    await user.keyboard("{Enter}");
    await within(dialog).findByText(/is a tag on/);

    await user.click(within(dialog).getByRole("button", { name: "Back" }));

    expect(within(dialog).queryByText(/is a tag on/)).toBeNull();
    // 「戻る」で確認のビューから入力のビューに戻ると、入力は作り直される
    // （確認のビューには入力が無い）ので、あらためて取得する。
    const inputAfterBack = within(dialog).getByRole("textbox", {
      name: "Add synonym",
    }) as HTMLInputElement;
    expect(inputAfterBack.value).toBe("anime");
    expect(document.activeElement).toBe(inputAfterBack);
    expect(server.tags.some((t) => t.id === 2 && t.name === "anime")).toBe(true);
    expect(server.tags.find((t) => t.id === 1)?.synonyms).toEqual([]);
  });

  it("確認の後に別のタブでその名前が別のタグへ移っていると、統合されずに確認がやり直しになる", async () => {
    const user = userEvent.setup();
    install();
    server.tags = [
      tag({ id: 1, name: "Anime", videoCount: 2 }),
      tag({ id: 2, name: "anime", videoCount: 10 }),
    ];
    renderPage();
    await screen.findByTitle("Anime");

    const row = screen.getByTitle("Anime").closest("div")!.parentElement!;
    await user.click(within(row).getByRole("button", { name: "Synonyms" }));
    const dialog = await screen.findByRole("dialog", { name: 'Synonyms of "Anime"' });
    const input = within(dialog).getByRole("textbox", { name: "Add synonym" });
    await user.type(input, "anime");
    await user.keyboard("{Enter}");
    await within(dialog).findByText(/is a tag on 10 videos/);

    // 確認を出したあと、別のタブで id2 を改名し、新しく「anime」（id3）を作る。
    server.tags[1]!.name = "anime-old";
    server.tags.push(tag({ id: 3, name: "anime", videoCount: 20 }));

    await user.click(within(dialog).getByRole("button", { name: "Merge" }));

    expect(await within(dialog).findByText(/is a tag on 20 videos/)).toBeDefined();
    expect(server.tags.some((t) => t.id === 2 && t.name === "anime-old")).toBe(true);
  });

  it("シノニムの窓でEscを押すと閉じ、フォーカスがその行の「シノニム」へ戻る", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("Anime");

    const row = screen.getByTitle("Anime").closest("div")!.parentElement!;
    const synonymsButton = within(row).getByRole("button", { name: "Synonyms" });
    await user.click(synonymsButton);
    await screen.findByRole("dialog", { name: 'Synonyms of "Anime"' });

    await user.keyboard("{Escape}");

    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    await waitFor(() => expect(document.activeElement).toBe(synonymsButton));
  });

  it("シノニムの追加の失敗は、入力を打ち直すと消える（N1）", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("Drama");

    const row = screen.getByTitle("Drama").closest("div")!.parentElement!;
    await user.click(within(row).getByRole("button", { name: "Synonyms" }));
    const dialog = await screen.findByRole("dialog", { name: 'Synonyms of "Drama"' });
    const input = within(dialog).getByRole("textbox", { name: "Add synonym" });
    await user.type(input, "アニメ");
    await user.keyboard("{Enter}");
    await within(dialog).findByText('"アニメ" is already a synonym of the tag "Anime".');

    await user.type(input, "2");

    expect(
      within(dialog).queryByText('"アニメ" is already a synonym of the tag "Anime".'),
    ).toBeNull();
  });

  it("シノニムを解除すると、次のチップの×、無ければ前、1つも無ければ入力へフォーカスが移る（N2）", async () => {
    const user = userEvent.setup();
    install();
    server.tags = [tag({ id: 1, name: "Anime", synonyms: ["A", "B", "C"] })];
    renderPage();
    await screen.findByTitle("Anime");

    const row = screen.getByTitle("Anime").closest("div")!.parentElement!;
    await user.click(within(row).getByRole("button", { name: "Synonyms" }));
    const dialog = await screen.findByRole("dialog", { name: 'Synonyms of "Anime"' });
    const input = within(dialog).getByRole("textbox", { name: "Add synonym" });

    // 中間（B）を解除すると、次（C）の×へ移る。
    await user.click(
      within(dialog).getByRole("button", { name: 'Remove the synonym "B"' }),
    );
    await waitFor(() => expect(within(dialog).queryByText("B")).toBeNull());
    await waitFor(() =>
      expect(document.activeElement).toBe(
        within(dialog).getByRole("button", { name: 'Remove the synonym "C"' }),
      ),
    );

    // 残りの最後（C）を解除すると、前（A）の×へ移る。
    await user.click(
      within(dialog).getByRole("button", { name: 'Remove the synonym "C"' }),
    );
    await waitFor(() => expect(within(dialog).queryByText("C")).toBeNull());
    await waitFor(() =>
      expect(document.activeElement).toBe(
        within(dialog).getByRole("button", { name: 'Remove the synonym "A"' }),
      ),
    );

    // 最後の1つ（A）を解除すると、入力へ移る。
    await user.click(
      within(dialog).getByRole("button", { name: 'Remove the synonym "A"' }),
    );
    await waitFor(() => expect(within(dialog).queryByText("A")).toBeNull());
    await waitFor(() => expect(document.activeElement).toBe(input));
  });

  it("シノニム登録に伴う統合が失敗すると、フォーカスが「統合する」へ戻る（N4）", async () => {
    const user = userEvent.setup();
    install();
    server.tags = [
      tag({ id: 1, name: "Anime", videoCount: 2 }),
      tag({ id: 2, name: "anime", videoCount: 10 }),
    ];
    renderPage();
    await screen.findByTitle("Anime");

    const row = screen.getByTitle("Anime").closest("div")!.parentElement!;
    await user.click(within(row).getByRole("button", { name: "Synonyms" }));
    const dialog = await screen.findByRole("dialog", { name: 'Synonyms of "Anime"' });
    const input = within(dialog).getByRole("textbox", { name: "Add synonym" });
    await user.type(input, "anime");
    await user.keyboard("{Enter}");
    await within(dialog).findByText(/is a tag on/);

    server.failNextSynonymsPost = true;
    const confirmButton = within(dialog).getByRole("button", { name: "Merge" });
    await user.click(confirmButton);

    expect(await within(dialog).findByRole("alert")).toBeDefined();
    await waitFor(() => expect(document.activeElement).toBe(confirmButton));
  });

  it("シノニム登録に伴う統合が成功すると、統合元がその場で一覧から消える（バックグラウンドの取り直しを待たない）", async () => {
    const user = userEvent.setup();
    install();
    server.tags = [
      tag({ id: 1, name: "Anime", videoCount: 2 }),
      tag({ id: 2, name: "anime", videoCount: 10 }),
    ];
    renderPage();
    await screen.findByTitle("Anime");
    await screen.findByTitle("anime");

    const row = screen.getByTitle("Anime").closest("div")!.parentElement!;
    await user.click(within(row).getByRole("button", { name: "Synonyms" }));
    const dialog = await screen.findByRole("dialog", { name: 'Synonyms of "Anime"' });
    const input = within(dialog).getByRole("textbox", { name: "Add synonym" });
    await user.type(input, "anime");
    await user.keyboard("{Enter}");
    await within(dialog).findByText(/is a tag on/);

    // バックグラウンドの取り直し（afterTagChanged の refreshTags）が失敗しても、
    // その場での反映（統合元を一覧から消す）は変わらないことを確かめる。
    server.failNextGet = true;

    await user.click(within(dialog).getByRole("button", { name: "Merge" }));
    await within(dialog).findByText("anime");
    await user.click(within(dialog).getByRole("button", { name: "Close" }));

    expect(screen.queryByTitle("anime")).toBeNull();
    expect(screen.getByTitle("Anime")).toBeDefined();
  });

  it("素のシノニムの登録・解除では、ほかのタグを一覧から消さない", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("Drama");

    const row = screen.getByTitle("Drama").closest("div")!.parentElement!;
    await user.click(within(row).getByRole("button", { name: "Synonyms" }));
    const dialog = await screen.findByRole("dialog", { name: 'Synonyms of "Drama"' });
    const input = within(dialog).getByRole("textbox", { name: "Add synonym" });
    await user.type(input, "ドラマ");
    await user.keyboard("{Enter}");
    await within(dialog).findByText("ドラマ");
    await user.click(within(dialog).getByRole("button", { name: "Close" }));

    expect(screen.getByTitle("旅行")).toBeDefined();
    expect(screen.getByTitle("Anime")).toBeDefined();
    expect(screen.getByTitle("Drama")).toBeDefined();
  });

  it("確認をとる時点で名前の持ち主がもう無ければ、1回だけ送り直して素のまま登録する", async () => {
    const user = userEvent.setup();
    install();
    server.tags = [tag({ id: 1, name: "Anime", videoCount: 2 })];
    server.forceMergeRequiredOnce = true;
    renderPage();
    await screen.findByTitle("Anime");

    const row = screen.getByTitle("Anime").closest("div")!.parentElement!;
    await user.click(within(row).getByRole("button", { name: "Synonyms" }));
    const dialog = await screen.findByRole("dialog", { name: 'Synonyms of "Anime"' });
    const input = within(dialog).getByRole("textbox", {
      name: "Add synonym",
    }) as HTMLInputElement;
    await user.type(input, "anime");
    await user.keyboard("{Enter}");

    // 確認には切り替わらず、素の登録として直接付く（送り直しが1回だけ
    // 許される。N6b）。
    expect(await within(dialog).findByText("anime")).toBeDefined();
    expect(within(dialog).queryByText(/is a tag on/)).toBeNull();
    expect(input.value).toBe("");
  });

  it("シノニムの追加が成功する応答の間に打ち直していたら、入力の文字を消さない", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("Drama");

    const row = screen.getByTitle("Drama").closest("div")!.parentElement!;
    await user.click(within(row).getByRole("button", { name: "Synonyms" }));
    const dialog = await screen.findByRole("dialog", { name: 'Synonyms of "Drama"' });
    const input = within(dialog).getByRole("textbox", {
      name: "Add synonym",
    }) as HTMLInputElement;
    await user.type(input, "ドラマ");

    holdNextMutation = true;
    await user.keyboard("{Enter}");
    await waitFor(() => expect(input.getAttribute("aria-busy")).toBe("true"));

    // 応答を待つ間に、別の名前へ打ち直す。
    await user.clear(input);
    await user.type(input, "別の下書き");

    release?.();

    await within(dialog).findByText("ドラマ");
    // 追加した「ドラマ」は付いたが、その後に打ち直した文字は消えない。
    expect(input.value).toBe("別の下書き");
  });

  it("並行して複数のシノニムを解除しても、互いの結果を巻き戻さない", async () => {
    const user = userEvent.setup();
    install();
    server.tags = [tag({ id: 1, name: "Anime", synonyms: ["A", "B"] })];
    renderPage();
    await screen.findByTitle("Anime");

    const row = screen.getByTitle("Anime").closest("div")!.parentElement!;
    await user.click(within(row).getByRole("button", { name: "Synonyms" }));
    const dialog = await screen.findByRole("dialog", { name: 'Synonyms of "Anime"' });

    // バックグラウンドの取り直し（各解除の afterTagChanged による
    // refreshTags）がサーバーの正しい状態で上書きして、クライアント側の
    // 巻き戻りを覆い隠してしまわないようにする。ここで確かめたいのは、
    // その場（サーバーの応答を待たない側）の反映が正しいことである。
    server.failAllGets = true;
    holdSynonymDeletes = true;
    await user.click(
      within(dialog).getByRole("button", { name: 'Remove the synonym "A"' }),
    );
    await user.click(
      within(dialog).getByRole("button", { name: 'Remove the synonym "B"' }),
    );
    expect(synonymDeleteReleases).toHaveLength(2);

    // 両方の応答を、同じタイミングで（間に描画を挟まず）解決させる。片方の
    // 結果がもう片方の閉じ込めた古い一覧で上書きされると、どちらか一方が
    // 生き残ってしまう。
    synonymDeleteReleases[0]!();
    synonymDeleteReleases[1]!();

    await waitFor(() => {
      expect(within(dialog).queryByText("A")).toBeNull();
      expect(within(dialog).queryByText("B")).toBeNull();
    });
  });
});

describe("TagsPage の英語の文言", () => {
  const userData = ["旅行", "Anime", "アニメ", "Drama", "存在しない語", "新規"];

  function rowOf(name: string): HTMLElement {
    return screen.getByTitle(name).closest("div")!.parentElement!;
  }

  it("疑似ロケールで、一覧・検索・作成・改名・メニューの文言がカタログから出る", async () => {
    enablePseudoLocale();
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("旅行");
    expectCatalogTextOnly(document.body, userData);

    const search = screen.getByRole("searchbox");
    await user.type(search, "ani");
    await waitFor(() => expect(screen.queryByTitle("旅行")).toBeNull());
    expectCatalogTextOnly(document.body, userData);

    await user.clear(search);
    await user.type(search, "存在しない語");
    await screen.findByRole("heading", { level: 2, name: /match/ });
    expectCatalogTextOnly(document.body, userData);
    await user.clear(search);
    await screen.findByTitle("旅行");

    // 作成の行（理由と、送信の失敗）。
    await user.click(screen.getByRole("button", { name: /New tag/ }));
    const create = await screen.findByRole("textbox", { name: /New tag name/ });
    await user.type(create, "x".repeat(101));
    expectCatalogTextOnly(document.body, userData);
    await user.clear(create);
    await user.type(create, "旅行");
    await user.keyboard("{Enter}");
    await screen.findByText(/already exists/);
    expectCatalogTextOnly(document.body, userData);
    await user.keyboard("{Escape}");

    // 改名の行と、その他の操作のメニュー。
    await user.click(within(rowOf("Anime")).getByRole("button", { name: /Rename/ }));
    await screen.findByRole("textbox", { name: /New name for/ });
    expectCatalogTextOnly(document.body, userData);
    await user.keyboard("{Escape}");

    await user.click(
      within(rowOf("Anime")).getByRole("button", { name: /More actions/ }),
    );
    await screen.findByRole("menu");
    expectCatalogTextOnly(document.body, userData);
  });

  it("疑似ロケールで、削除・統合・シノニムの窓の文言がカタログから出る", async () => {
    enablePseudoLocale();
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("Anime");

    await user.click(
      within(rowOf("Anime")).getByRole("button", { name: /More actions/ }),
    );
    await user.click(await screen.findByRole("menuitem", { name: /Delete/ }));
    await screen.findByRole("dialog");
    expectCatalogTextOnly(document.body, userData);
    await user.keyboard("{Escape}");
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());

    await user.click(within(rowOf("旅行")).getByRole("button", { name: /More actions/ }));
    await user.click(await screen.findByRole("menuitem", { name: /Merge into/ }));
    const merge = await screen.findByRole("dialog");
    const combo = within(merge).getByRole("combobox");
    await user.type(combo, "アニ");
    await within(merge).findByRole("option", { name: /Anime/ });
    expectCatalogTextOnly(document.body, userData);
    await user.click(within(merge).getByRole("option", { name: /Anime/ }));
    expectCatalogTextOnly(document.body, userData);
    server.failNextMerge = true;
    await user.click(within(merge).getByRole("button", { name: /^⟦Merge⟧$/ }));
    await within(merge).findByRole("alert");
    expectCatalogTextOnly(document.body, userData);
    await user.click(within(merge).getByRole("button", { name: /Cancel/ }));
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());

    await user.click(
      within(rowOf("Anime")).getByRole("button", { name: /^⟦Synonyms⟧$/ }),
    );
    const synonyms = await screen.findByRole("dialog");
    const input = within(synonyms).getByRole("textbox");
    await user.type(input, "Anime");
    await user.keyboard("{Enter}");
    await within(synonyms).findByText(/this tag's name/);
    expectCatalogTextOnly(document.body, userData);

    await user.clear(input);
    await user.type(input, "Drama");
    await user.keyboard("{Enter}");
    await within(synonyms).findByText(/is a tag on/);
    expectCatalogTextOnly(document.body, userData);
  });

  it("疑似ロケールで、空・読み込み中・失敗の状態の文言がカタログから出る", async () => {
    enablePseudoLocale();
    install();
    holdGetsFrom = 1;
    const loading = renderPage();
    await screen.findByRole("status");
    expectCatalogTextOnly(document.body, userData);
    loading.unmount();
    holdGetsFrom = null;
    heldGetReleases.length = 0;

    __resetTagsForTest();
    server.failAllGets = true;
    const failed = renderPage();
    await screen.findByRole("heading", { level: 2, name: /load/ });
    expectCatalogTextOnly(document.body, userData);
    failed.unmount();

    __resetTagsForTest();
    server.failAllGets = false;
    server.tags = [];
    renderPage();
    await screen.findByRole("heading", { level: 2, name: /No tags yet/ });
    expectCatalogTextOnly(document.body, userData);
  });

  it("件数と本数は 1 と複数で単数・複数を分ける", async () => {
    const user = userEvent.setup();
    install();
    server.tags = [tag({ id: 1, name: "旅行", videoCount: 1 })];
    renderPage();
    await screen.findByTitle("旅行");
    expect(screen.getByText("1 tag")).toBeDefined();
    expect(rowOf("旅行").textContent).toContain("1 video");
    expect(rowOf("旅行").textContent).not.toContain("1 videos");

    await user.click(within(rowOf("旅行")).getByRole("button", { name: "More actions" }));
    await user.click(await screen.findByRole("menuitem", { name: "Delete…" }));
    const dialog = await screen.findByRole("dialog", { name: 'Delete "旅行"' });
    expect(
      within(dialog).getByText(
        "This tag will be removed from 1 video. This can't be undone.",
      ),
    ).toBeDefined();
  });

  it("日本語のタグ名とシノニムはそのまま表示され、その綴りで検索できる", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("旅行");
    expect(
      screen.getByRole("link", { name: "Open the library filtered by 旅行" }),
    ).toBeDefined();
    expect(screen.getByText("Synonyms: アニメ")).toBeDefined();

    const search = screen.getByRole("searchbox", { name: "Search tags" });
    await user.type(search, "旅");
    await waitFor(() => expect(screen.queryByTitle("Anime")).toBeNull());
    expect(screen.getByTitle("旅行")).toBeDefined();
    expect(screen.getByText("1 of 3 tags")).toBeDefined();

    await user.clear(search);
    await user.type(search, "アニメ");
    await waitFor(() => expect(screen.queryByTitle("旅行")).toBeNull());
    expect(screen.getByTitle("Anime")).toBeDefined();
  });

  it("空のタグ名と長すぎるタグ名のサーバーの拒否を、上限を埋め込んだ英語で出す", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("旅行");

    await user.click(screen.getByRole("button", { name: "New tag" }));
    const input = await screen.findByRole("textbox", { name: "New tag name" });
    server.nextError = {
      status: 400,
      body: {
        code: "invalid_request",
        reason: "tag_name_empty",
        message: "server-side message",
      },
    };
    await user.type(input, "新規");
    await user.keyboard("{Enter}");
    expect((await screen.findByRole("alert")).textContent).toBe("Enter a tag name.");

    server.nextError = {
      status: 400,
      body: {
        code: "invalid_request",
        reason: "tag_name_too_long",
        limit: 100,
        message: "server-side message",
      },
    };
    await user.type(input, "2");
    await user.keyboard("{Enter}");
    expect(
      await screen.findByText("Use a tag name of 100 characters or fewer."),
    ).toBeDefined();
  });

  it("自分の名前・自分のシノニムの登録は、このタグのことだと分かる英語で出す", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("Anime");

    await user.click(within(rowOf("Anime")).getByRole("button", { name: "Synonyms" }));
    const dialog = await screen.findByRole("dialog", { name: 'Synonyms of "Anime"' });
    const input = within(dialog).getByRole("textbox", { name: "Add synonym" });
    await user.type(input, "Anime");
    await user.keyboard("{Enter}");
    expect(
      await within(dialog).findByText('"Anime" is already this tag\'s name.'),
    ).toBeDefined();
  });

  it("統合が要る名前の確認は、API の tagName のタグ名で説明する", async () => {
    const user = userEvent.setup();
    install();
    server.tags = [
      tag({ id: 1, name: "Anime", videoCount: 2 }),
      tag({ id: 2, name: "ドラマ", synonyms: ["drama"], videoCount: 1 }),
    ];
    renderPage();
    await screen.findByTitle("Anime");

    await user.click(within(rowOf("Anime")).getByRole("button", { name: "Synonyms" }));
    const dialog = await screen.findByRole("dialog", { name: 'Synonyms of "Anime"' });
    const input = within(dialog).getByRole("textbox", { name: "Add synonym" });
    // サーバーは整えた名前（前後の空白を除くだけ）を BINARY で照合するので、
    // 前後に空白を付けて打った綴りでも、tagName は元の名前「ドラマ」になる。
    await user.type(input, "  ドラマ  ");
    await user.keyboard("{Enter}");
    expect(
      await within(dialog).findByText(
        '"ドラマ" is a tag on 1 video. Merging it into "Anime" adds "Anime" to those videos, and "ドラマ" and its synonyms become synonyms of "Anime". "ドラマ" leaves the tag list.',
      ),
    ).toBeDefined();
  });

  it("同じタグへの統合の拒否を英語で出す", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("旅行");

    await user.click(within(rowOf("旅行")).getByRole("button", { name: "More actions" }));
    await user.click(
      await screen.findByRole("menuitem", { name: "Merge into another tag…" }),
    );
    const dialog = await screen.findByRole("dialog", { name: 'Merge "旅行"' });
    await user.type(within(dialog).getByRole("combobox"), "Anime");
    await user.click(await within(dialog).findByRole("option", { name: /Anime/ }));
    server.nextError = {
      status: 400,
      body: {
        code: "invalid_request",
        reason: "merge_same_tag",
        message: "server-side message",
      },
    };
    await user.click(within(dialog).getByRole("button", { name: "Merge" }));
    expect((await within(dialog).findByRole("alert")).textContent).toBe(
      "A tag can't be merged into itself.",
    );
  });
});

describe("TagsPage 仮のタグ", () => {
  function rowOf(name: string): HTMLElement {
    return screen.getByTitle(name).closest("div")!.parentElement!;
  }

  function tentativeOnlyButton(): HTMLElement {
    return screen.getByRole("button", { name: "Tentative only" });
  }

  beforeEach(() => {
    server.tags = [
      tag({ id: 1, name: "Alpha", tentative: true, videoCount: 2 }),
      tag({ id: 2, name: "Beta", tentative: true }),
      tag({ id: 3, name: "Cat", tentative: true }),
      tag({ id: 4, name: "Gamma", synonyms: ["ガンマ"], videoCount: 5 }),
    ];
  });

  it("仮のタグの行は目印と「確定する」を持ち、確定したタグの行は持たない（受け入れ条件5）", async () => {
    install();
    renderPage();
    await screen.findByTitle("Alpha");

    const alpha = rowOf("Alpha");
    expect(within(alpha).getByText("Tentative")).toBeDefined();
    expect(within(alpha).getByRole("button", { name: "Confirm" })).toBeDefined();

    const gamma = rowOf("Gamma");
    expect(within(gamma).queryByText("Tentative")).toBeNull();
    expect(within(gamma).queryByRole("button", { name: "Confirm" })).toBeNull();
    // 確定した行の操作は今と同じ3つ。
    expect(within(gamma).getAllByRole("button")).toHaveLength(3);
  });

  it("「Tentative only」で仮のタグだけが並び、件数は全タグを分母にする（受け入れ条件6）", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("Gamma");

    const toggle = tentativeOnlyButton();
    expect(toggle.getAttribute("aria-pressed")).toBe("false");
    await user.click(toggle);

    expect(toggle.getAttribute("aria-pressed")).toBe("true");
    expect(await screen.findByText("3 of 4 tags")).toBeDefined();
    expect(screen.queryByTitle("Gamma")).toBeNull();
    expect(screen.getByTitle("Alpha")).toBeDefined();

    // 検索と重ねられる。
    await user.type(screen.getByRole("searchbox", { name: "Search tags" }), "be");
    expect(await screen.findByText("1 of 4 tags")).toBeDefined();
    expect(screen.getByTitle("Beta")).toBeDefined();
    expect(screen.queryByTitle("Alpha")).toBeNull();

    await user.click(toggle);
    expect(await screen.findByText("1 of 4 tags")).toBeDefined();
  });

  it("確定するとその行が確定したタグになり、同じ行の「改名」へフォーカスが移る（受け入れ条件7）", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("Alpha");

    await user.click(within(rowOf("Alpha")).getByRole("button", { name: "Confirm" }));

    expect(await screen.findByText('Confirmed "Alpha"')).toBeDefined();
    await waitFor(() =>
      expect(
        within(rowOf("Alpha")).queryByRole("button", { name: "Confirm" }),
      ).toBeNull(),
    );
    expect(within(rowOf("Alpha")).queryByText("Tentative")).toBeNull();
    await waitFor(() =>
      expect(document.activeElement).toBe(
        within(rowOf("Alpha")).getByRole("button", { name: "Rename" }),
      ),
    );
    await user.click(
      within(rowOf("Alpha")).getByRole("button", { name: "More actions" }),
    );
    expect(await screen.findByRole("menuitem", { name: "Delete…" })).toBeDefined();
  });

  it("確定の送信中は二度押しを無視し、同じ行の「改名」「その他の操作」がdisabledになる", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("Alpha");

    holdConfirms = true;
    const confirm = within(rowOf("Alpha")).getByRole("button", { name: "Confirm" });
    await user.click(confirm);
    await user.click(confirm);
    await user.keyboard("{Enter}");

    expect(server.confirmCalls).toBe(1);
    expect(confirm.getAttribute("aria-busy")).toBe("true");
    expect(confirm.getAttribute("aria-disabled")).toBe("true");
    expect(confirm.hasAttribute("disabled")).toBe(false);
    const alpha = rowOf("Alpha");
    expect(
      within(alpha).getByRole("button", { name: "Rename" }).hasAttribute("disabled"),
    ).toBe(true);
    expect(
      within(alpha)
        .getByRole("button", { name: "More actions" })
        .hasAttribute("disabled"),
    ).toBe(true);
    // ほかの行はそのまま。
    expect(
      within(rowOf("Beta"))
        .getByRole("button", { name: "Rename" })
        .hasAttribute("disabled"),
    ).toBe(false);

    confirmReleases.forEach((resolve) => resolve());
    expect(await screen.findByText('Confirmed "Alpha"')).toBeDefined();
    expect(server.confirmCalls).toBe(1);
  });

  it("「Tentative only」中に確定すると、その行が外れて次の行の「改名」へフォーカスが移る", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("Alpha");
    await user.click(tentativeOnlyButton());

    await user.click(within(rowOf("Alpha")).getByRole("button", { name: "Confirm" }));

    await waitFor(() => expect(screen.queryByTitle("Alpha")).toBeNull());
    await waitFor(() =>
      expect(document.activeElement).toBe(
        within(rowOf("Beta")).getByRole("button", { name: "Rename" }),
      ),
    );
    expect(screen.getByText("2 of 4 tags")).toBeDefined();
  });

  it("仮のタグの行のメニューは「削除…」の代わりに「却下する…」を持ち、確定したタグの行は今のまま（受け入れ条件12）", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("Alpha");

    await user.click(
      within(rowOf("Alpha")).getByRole("button", { name: "More actions" }),
    );
    const items = await screen.findAllByRole("menuitem");
    expect(items.map((item) => item.textContent)).toEqual([
      "Merge into another tag…",
      "Reject…",
    ]);
    await user.keyboard("{Escape}");
    await waitFor(() => expect(screen.queryByRole("menu")).toBeNull());

    await user.click(
      within(rowOf("Gamma")).getByRole("button", { name: "More actions" }),
    );
    const confirmedItems = await screen.findAllByRole("menuitem");
    expect(confirmedItems.map((item) => item.textContent)).toEqual([
      "Merge into another tag…",
      "Delete…",
    ]);
  });

  it("却下すると行が消えて却下した名前の一覧に出る（受け入れ条件8）", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("Alpha");

    const heading = screen.getByRole("button", { name: /Rejected names/ });
    expect(heading.getAttribute("aria-expanded")).toBe("false");
    await waitFor(() => expect(heading.textContent).toContain("0"));

    await user.click(
      within(rowOf("Alpha")).getByRole("button", { name: "More actions" }),
    );
    await user.click(await screen.findByRole("menuitem", { name: "Reject…" }));
    const dialog = await screen.findByRole("dialog", { name: 'Reject "Alpha"' });
    expect(
      within(dialog).getByText(
        'This tag will be removed from 2 videos, and automatic tagging won\'t create "Alpha" again. You can allow the name again from the rejected names below.',
      ),
    ).toBeDefined();
    await user.click(within(dialog).getByRole("button", { name: "Reject" }));

    await waitFor(() => expect(screen.queryByTitle("Alpha")).toBeNull());
    expect(await screen.findByText('Rejected "Alpha"')).toBeDefined();
    await waitFor(() =>
      expect(document.activeElement).toBe(
        within(rowOf("Beta")).getByRole("button", { name: "Rename" }),
      ),
    );
    await waitFor(() => expect(heading.textContent).toContain("1"));

    await user.click(heading);
    expect(heading.getAttribute("aria-expanded")).toBe("true");
    const list = screen.getByRole("list", { name: "Rejected names" });
    expect(within(list).getByTitle("Alpha")).toBeDefined();
  });

  it("0本の仮のタグの却下の確認は、どの動画にも付いていないと出す", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("Beta");

    await user.click(within(rowOf("Beta")).getByRole("button", { name: "More actions" }));
    await user.click(await screen.findByRole("menuitem", { name: "Reject…" }));
    const dialog = await screen.findByRole("dialog", { name: 'Reject "Beta"' });
    expect(
      within(dialog).getByText(
        "This tag isn't on any videos. Automatic tagging won't create \"Beta\" again. You can allow the name again from the rejected names below.",
      ),
    ).toBeDefined();

    await user.keyboard("{Escape}");
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    await waitFor(() =>
      expect(document.activeElement).toBe(
        within(rowOf("Beta")).getByRole("button", { name: "More actions" }),
      ),
    );
  });

  it("却下の前に別のタブで確定されていると、窓を閉じて一覧を取り直す（Edge Case「操作の競合」）", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("Alpha");

    await user.click(
      within(rowOf("Alpha")).getByRole("button", { name: "More actions" }),
    );
    await user.click(await screen.findByRole("menuitem", { name: "Reject…" }));
    const dialog = await screen.findByRole("dialog", { name: 'Reject "Alpha"' });

    // 窓を開いたあと、別のタブで先に確定されたことにする。
    server.tags = server.tags.map((item) =>
      item.id === 1 ? { ...item, tentative: false } : item,
    );
    const getsBefore = server.getCalls;
    await user.click(within(dialog).getByRole("button", { name: "Reject" }));

    expect(
      await screen.findByText("This tag was already confirmed, so the list was reloaded"),
    ).toBeDefined();
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(server.getCalls).toBeGreaterThan(getsBefore);
    await waitFor(() =>
      expect(
        within(rowOf("Alpha")).queryByRole("button", { name: "Confirm" }),
      ).toBeNull(),
    );
    expect(server.rejectedNames).toEqual([]);
  });

  it("却下の一般の失敗は窓の中のalertで伝え、窓は開いたまま", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("Alpha");

    await user.click(
      within(rowOf("Alpha")).getByRole("button", { name: "More actions" }),
    );
    await user.click(await screen.findByRole("menuitem", { name: "Reject…" }));
    const dialog = await screen.findByRole("dialog", { name: 'Reject "Alpha"' });
    server.nextError = { status: 500, body: { code: "internal", message: "x" } };
    await user.click(within(dialog).getByRole("button", { name: "Reject" }));

    expect(await within(dialog).findByRole("alert")).toBeDefined();
    expect(screen.getByRole("dialog", { name: 'Reject "Alpha"' })).toBeDefined();
  });

  it("却下した名前を×で一覧から外せる。最後の1つなら見出しへフォーカスが移る（受け入れ条件13）", async () => {
    const user = userEvent.setup();
    server.rejectedNames = ["Old", "Stale"];
    install();
    renderPage();
    await screen.findByTitle("Alpha");

    const heading = screen.getByRole("button", { name: /Rejected names/ });
    await user.click(heading);
    const list = await screen.findByRole("list", { name: "Rejected names" });
    expect(
      screen.getByText(
        "Automatic tagging won't create these tags. Remove a name to allow it again.",
      ),
    ).toBeDefined();

    await user.click(within(list).getByRole("button", { name: 'Allow "Old" again' }));
    await waitFor(() => expect(within(list).queryByTitle("Old")).toBeNull());
    expect(server.rejectedNames).toEqual(["Stale"]);
    await waitFor(() =>
      expect(document.activeElement).toBe(
        within(list).getByRole("button", { name: 'Allow "Stale" again' }),
      ),
    );

    await user.click(within(list).getByRole("button", { name: 'Allow "Stale" again' }));
    expect(await screen.findByText("No rejected names")).toBeDefined();
    await waitFor(() => expect(document.activeElement).toBe(heading));
    expect(heading.textContent).toContain("0");
  });

  it("却下した名前を取れなかったら理由を出し、再試行できる", async () => {
    const user = userEvent.setup();
    server.failRejectedGets = true;
    server.rejectedNames = ["Old"];
    install();
    renderPage();
    await screen.findByTitle("Alpha");

    await user.click(screen.getByRole("button", { name: /Rejected names/ }));
    expect(await screen.findByText("Couldn't load the rejected names")).toBeDefined();

    server.failRejectedGets = false;
    await user.click(screen.getByRole("button", { name: "Retry" }));
    expect(
      await screen.findByRole("button", { name: 'Allow "Old" again' }),
    ).toBeDefined();
  });

  it("取り外しの前から待っていた取り直しの古い応答で、外した名前が戻らない", async () => {
    const user = userEvent.setup();
    server.rejectedNames = ["Old", "Stale"];
    install();
    renderPage();
    await screen.findByTitle("Alpha");
    await user.click(screen.getByRole("button", { name: /Rejected names/ }));
    const list = await screen.findByRole("list", { name: "Rejected names" });

    // 却下で取り直しが始まり、その応答は取り外しの前の並びを持ったまま止まる。
    holdRejectedGets = true;
    await user.click(within(rowOf("Beta")).getByRole("button", { name: "More actions" }));
    await user.click(await screen.findByRole("menuitem", { name: "Reject…" }));
    await user.click(await screen.findByRole("button", { name: "Reject" }));
    await waitFor(() => expect(screen.queryByTitle("Beta")).toBeNull());
    expect(rejectedGetReleases).toHaveLength(1);
    holdRejectedGets = false;

    await user.click(within(list).getByRole("button", { name: 'Allow "Old" again' }));
    await waitFor(() => expect(within(list).queryByTitle("Old")).toBeNull());

    rejectedGetReleases.forEach((resolve) => resolve());
    // 取り外しのあとの取り直しが却下した Beta を並べ、古い応答の Old は戻らない。
    expect(
      await within(list).findByRole("button", { name: 'Allow "Beta" again' }),
    ).toBeDefined();
    await new Promise((resolve) => setTimeout(resolve, 0));
    expect(within(list).queryByTitle("Old")).toBeNull();
  });

  it("一覧を持ったあとの取り直しに失敗しても、古い並びを残さず理由と再試行を出す", async () => {
    const user = userEvent.setup();
    server.rejectedNames = ["Old"];
    install();
    renderPage();
    await screen.findByTitle("Alpha");
    await user.click(screen.getByRole("button", { name: /Rejected names/ }));
    expect(
      await screen.findByRole("button", { name: 'Allow "Old" again' }),
    ).toBeDefined();

    server.failRejectedGets = true;
    await user.click(within(rowOf("Beta")).getByRole("button", { name: "More actions" }));
    await user.click(await screen.findByRole("menuitem", { name: "Reject…" }));
    await user.click(await screen.findByRole("button", { name: "Reject" }));

    expect(await screen.findByText("Couldn't load the rejected names")).toBeDefined();
    expect(screen.queryByRole("button", { name: 'Allow "Old" again' })).toBeNull();

    server.failRejectedGets = false;
    await user.click(screen.getByRole("button", { name: "Retry" }));
    expect(
      await screen.findByRole("button", { name: 'Allow "Beta" again' }),
    ).toBeDefined();
  });

  it("「Tentative only」中に作成を取り消すと、「新しいタグ」へフォーカスが戻る", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("Alpha");
    await user.click(tentativeOnlyButton());

    const newTag = screen.getByRole("button", { name: "New tag" });
    await user.click(newTag);
    await user.click(screen.getByRole("button", { name: "Cancel" }));

    await waitFor(() => expect(document.activeElement).toBe(newTag));
  });

  it("確定の送信中に「Tentative only」を外すと、確定した行はそのまま出て、その行の「改名」へフォーカスが移る", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("Alpha");
    await user.click(tentativeOnlyButton());

    holdConfirms = true;
    await user.click(within(rowOf("Alpha")).getByRole("button", { name: "Confirm" }));
    await user.click(tentativeOnlyButton());
    expect(tentativeOnlyButton().getAttribute("aria-pressed")).toBe("false");

    confirmReleases.forEach((resolve) => resolve());
    expect(await screen.findByText('Confirmed "Alpha"')).toBeDefined();
    await waitFor(() =>
      expect(document.activeElement).toBe(
        within(rowOf("Alpha")).getByRole("button", { name: "Rename" }),
      ),
    );
  });

  it("「Tentative only」中に並行した確定が逆の順で終わっても、残った次の行へフォーカスが移る", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("Alpha");
    await user.click(tentativeOnlyButton());

    holdConfirms = true;
    await user.click(within(rowOf("Alpha")).getByRole("button", { name: "Confirm" }));
    await user.click(within(rowOf("Beta")).getByRole("button", { name: "Confirm" }));
    expect(confirmReleases).toHaveLength(2);

    // 後から押した Beta が先に終わり、そのあとで Alpha が終わる。
    const [releaseAlpha, releaseBeta] = confirmReleases;
    releaseBeta!();
    await waitFor(() => expect(screen.queryByTitle("Beta")).toBeNull());
    releaseAlpha!();
    await waitFor(() => expect(screen.queryByTitle("Alpha")).toBeNull());

    await waitFor(() =>
      expect(document.activeElement).toBe(
        within(rowOf("Cat")).getByRole("button", { name: "Rename" }),
      ),
    );
  });

  it("「新しいタグ」で却下した名前を作ると、却下した名前の一覧から消える（受け入れ条件14）", async () => {
    const user = userEvent.setup();
    server.rejectedNames = ["Old"];
    install();
    renderPage();
    await screen.findByTitle("Alpha");

    await user.click(screen.getByRole("button", { name: /Rejected names/ }));
    expect(
      await screen.findByRole("button", { name: 'Allow "Old" again' }),
    ).toBeDefined();

    await user.click(screen.getByRole("button", { name: "New tag" }));
    await user.type(screen.getByRole("textbox", { name: "New tag name" }), "Old");
    await user.keyboard("{Enter}");

    expect(await screen.findByText("No rejected names")).toBeDefined();
    expect(screen.queryByRole("button", { name: 'Allow "Old" again' })).toBeNull();
  });

  it("「Tentative only」中の仮のタグどうしの統合は、「新しいタグ」へ落ちず次の行へフォーカスが移る", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("Alpha");
    await user.click(tentativeOnlyButton());

    await user.click(
      within(rowOf("Alpha")).getByRole("button", { name: "More actions" }),
    );
    await user.click(
      await screen.findByRole("menuitem", { name: "Merge into another tag…" }),
    );
    const dialog = await screen.findByRole("dialog", { name: 'Merge "Alpha"' });
    await user.type(
      within(dialog).getByRole("combobox", { name: "Tag to merge into" }),
      "Beta",
    );
    await user.click(await within(dialog).findByRole("option", { name: /Beta/ }));
    await user.click(within(dialog).getByRole("button", { name: "Merge" }));

    expect(await screen.findByText('Merged "Alpha" into "Beta"')).toBeDefined();
    // 統合元（Alpha）は消え、統合先（Beta）は確定になって絞り込みから外れる。
    await waitFor(() => expect(screen.queryByTitle("Beta")).toBeNull());
    await waitFor(() =>
      expect(document.activeElement).toBe(
        within(rowOf("Cat")).getByRole("button", { name: "Rename" }),
      ),
    );
  });

  it("「Tentative only」中に唯一のタグを却下すると、「Tentative only」が押せるまま、そこにフォーカスがある", async () => {
    const user = userEvent.setup();
    server.tags = [tag({ id: 1, name: "Alpha", tentative: true })];
    install();
    renderPage();
    await screen.findByTitle("Alpha");
    const toggle = tentativeOnlyButton();
    await user.click(toggle);

    await user.click(
      within(rowOf("Alpha")).getByRole("button", { name: "More actions" }),
    );
    await user.click(await screen.findByRole("menuitem", { name: "Reject…" }));
    const dialog = await screen.findByRole("dialog", { name: 'Reject "Alpha"' });
    await user.click(within(dialog).getByRole("button", { name: "Reject" }));

    expect(await screen.findByText("No tentative tags")).toBeDefined();
    expect(toggle.hasAttribute("disabled")).toBe(false);
    await waitFor(() => expect(document.activeElement).toBe(toggle));

    // 「Show all tags」で外すとタグが 0 なので、フォーカスは「新しいタグ」へ。
    await user.click(screen.getByRole("button", { name: "Show all tags" }));
    expect(await screen.findByText("No tags yet")).toBeDefined();
    expect(toggle.hasAttribute("disabled")).toBe(true);
    await waitFor(() =>
      expect(document.activeElement).toBe(
        screen.getAllByRole("button", { name: "New tag" })[0],
      ),
    );
  });

  it("「Tentative only」中に唯一の仮のタグへシノニムを足すと、入力にフォーカスが残り、閉じると「Tentative only」へ移る", async () => {
    const user = userEvent.setup();
    server.tags = [
      tag({ id: 1, name: "Alpha", tentative: true }),
      tag({ id: 4, name: "Gamma" }),
    ];
    install();
    renderPage();
    await screen.findByTitle("Alpha");
    const toggle = tentativeOnlyButton();
    await user.click(toggle);

    await user.click(within(rowOf("Alpha")).getByRole("button", { name: "Synonyms" }));
    const dialog = await screen.findByRole("dialog", { name: 'Synonyms of "Alpha"' });
    const input = within(dialog).getByRole("textbox", { name: "Add synonym" });
    await user.type(input, "アルファ");
    await user.keyboard("{Enter}");

    expect(await within(dialog).findByText("アルファ")).toBeDefined();
    expect(document.activeElement).toBe(input);

    await user.keyboard("{Escape}");
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(screen.queryByTitle("Alpha")).toBeNull();
    await waitFor(() => expect(document.activeElement).toBe(toggle));
  });

  it("「Tentative only」と検索で一致が無いときは、両方を外して検索の入力へ戻れる", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("Alpha");
    await user.click(tentativeOnlyButton());
    const search = screen.getByRole("searchbox", { name: "Search tags" });
    await user.type(search, "Gamma");

    expect(await screen.findByText('No tentative tags match "Gamma"')).toBeDefined();
    await user.click(screen.getByRole("button", { name: "Show all tags" }));
    await waitFor(() => expect(document.activeElement).toBe(search));
    expect(tentativeOnlyButton().getAttribute("aria-pressed")).toBe("false");
    expect(await screen.findByText("4 tags")).toBeDefined();
  });

  it("疑似ロケールで、仮のタグの行・絞り込み・却下の窓・却下した名前の文言がカタログから出る", async () => {
    enablePseudoLocale();
    const user = userEvent.setup();
    server.rejectedNames = ["Old"];
    install();
    renderPage();
    await screen.findByTitle("Alpha");
    const userData = ["Alpha", "Beta", "Cat", "Gamma", "ガンマ", "Old"];
    expectCatalogTextOnly(document.body, userData);

    await user.click(screen.getByRole("button", { name: /Rejected names/ }));
    await screen.findByRole("list");
    expectCatalogTextOnly(document.body, userData);

    await user.click(screen.getByRole("button", { name: /Tentative only/ }));
    await waitFor(() => expect(screen.queryByTitle("Gamma")).toBeNull());
    expectCatalogTextOnly(document.body, userData);

    await user.click(
      within(rowOf("Alpha")).getByRole("button", { name: /More actions/ }),
    );
    await screen.findByRole("menu");
    expectCatalogTextOnly(document.body, userData);
    await user.click(screen.getByRole("menuitem", { name: /Reject/ }));
    await screen.findByRole("dialog");
    expectCatalogTextOnly(document.body, userData);
  });
});

describe("TagsPage 見えている行だけ描く", () => {
  /** jsdom には表示域の高さと要素の高さが無いので、試験用の高さを置く。 */
  const rowHeight = 40;
  const viewportHeight = 200;
  const tagCount = 60;
  let originalInnerHeight = 0;
  let originalOffsetHeight: PropertyDescriptor | undefined;

  function name(index: number): string {
    return `Tag ${String(index).padStart(2, "0")}`;
  }

  function drawnIndexes(): number[] {
    return [...document.querySelectorAll<HTMLElement>("[data-index]")].map((row) =>
      Number(row.dataset.index),
    );
  }

  function wrapperOf(index: number): HTMLElement {
    return document.querySelector<HTMLElement>(`[data-index="${String(index)}"]`)!;
  }

  beforeEach(() => {
    server.tags = Array.from({ length: tagCount }, (_, index) =>
      tag({ id: index + 1, name: name(index), tentative: true }),
    );
    originalInnerHeight = window.innerHeight;
    window.innerHeight = viewportHeight;
    originalOffsetHeight = Object.getOwnPropertyDescriptor(
      HTMLElement.prototype,
      "offsetHeight",
    );
    Object.defineProperty(HTMLElement.prototype, "offsetHeight", {
      configurable: true,
      get(this: HTMLElement) {
        return this.hasAttribute("data-index") ? rowHeight : 0;
      },
    });
  });

  afterEach(() => {
    window.innerHeight = originalInnerHeight;
    if (originalOffsetHeight !== undefined) {
      Object.defineProperty(HTMLElement.prototype, "offsetHeight", originalOffsetHeight);
    }
  });

  /** tabPastRange は描いている範囲の最後の行の最後の操作から Tab で次の行へ進む。 */
  async function tabPastRange(user: ReturnType<typeof userEvent.setup>) {
    const last = Math.max(...drawnIndexes());
    expect(last).toBeLessThan(tagCount - 1);
    const more = within(wrapperOf(last)).getByRole("button", { name: "More actions" });
    more.focus();
    await user.tab();
    return last + 1;
  }

  it("全件ではなく表示域と前後の行だけを描き、件数は全件を数える", async () => {
    install();
    renderPage();
    await screen.findByTitle(name(0));
    expect(screen.getByText(`${String(tagCount)} tags`)).toBeDefined();
    const drawn = drawnIndexes();
    expect(drawn.length).toBeGreaterThan(0);
    expect(drawn.length).toBeLessThan(tagCount);
    expect(screen.queryByTitle(name(tagCount - 1))).toBeNull();
  });

  it("描いている範囲の端の Tab は次の行へ進み、Shift+Tab は前の行の最後の操作へ戻る", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle(name(0));

    const next = await tabPastRange(user);
    expect(document.activeElement).toBe(
      screen.getByRole("link", { name: `Open the library filtered by ${name(next)}` }),
    );

    // その行の最後の操作から、まだ描いていない次の行へ。
    within(wrapperOf(next)).getByRole("button", { name: "More actions" }).focus();
    await user.tab();
    expect(document.activeElement).toBe(screen.getByTitle(name(next + 1)));
    // フォーカスを移した行だけを描き続け、前の行は描いていない。
    expect(document.querySelector(`[data-index="${String(next)}"]`)).toBeNull();

    await user.tab({ shift: true });
    expect(document.activeElement).toBe(
      within(wrapperOf(next)).getByRole("button", { name: "More actions" }),
    );
  });

  it("フォーカスが一覧の外へ出ると、表示域の外の行を描き続けるのをやめる", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle(name(0));
    const next = await tabPastRange(user);

    // 同じ行の中の移動では描き続ける。
    await user.tab();
    expect(document.activeElement?.closest("[data-index]")).toBe(wrapperOf(next));
    await new Promise((resolve) => setTimeout(resolve, 0));
    expect(document.querySelector(`[data-index="${String(next)}"]`)).not.toBeNull();

    // 一覧の外（ツールバー）へ移ると、その行は Tab の順に残らない。
    await user.click(screen.getByRole("searchbox", { name: "Search tags" }));
    await waitFor(() =>
      expect(document.querySelector(`[data-index="${String(next)}"]`)).toBeNull(),
    );
  });

  it("作ったタグが描いている範囲の外に並ぶときも、その行を描いて名前へフォーカスが移る", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle(name(0));

    await user.click(screen.getByRole("button", { name: "New tag" }));
    await user.type(screen.getByRole("textbox", { name: "New tag name" }), "Tag 99");
    await user.keyboard("{Enter}");

    await waitFor(() =>
      expect(document.activeElement).toBe(
        screen.getByRole("link", { name: "Open the library filtered by Tag 99" }),
      ),
    );
  });

  it("全件の最後の行からの Tab は既定のまま一覧の外へ進む", async () => {
    const user = userEvent.setup();
    server.tags = server.tags.slice(0, 3);
    install();
    renderPage();
    await screen.findByTitle(name(2));
    const more = within(wrapperOf(2)).getByRole("button", { name: "More actions" });
    more.focus();
    await user.tab();
    expect(document.activeElement).not.toBe(more);
    expect(document.activeElement?.closest("[data-index]")).toBeNull();
  });

  it("削除で行が消えると、描いていなかった次の行を描いてその「改名」へフォーカスが移る", async () => {
    const user = userEvent.setup();
    server.tags = server.tags.map((item) => ({ ...item, tentative: false }));
    install();
    renderPage();
    await screen.findByTitle(name(0));
    const index = await tabPastRange(user);
    expect(document.querySelector(`[data-index="${String(index + 1)}"]`)).toBeNull();

    await user.click(
      within(wrapperOf(index)).getByRole("button", { name: "More actions" }),
    );
    await user.click(await screen.findByRole("menuitem", { name: "Delete…" }));
    const dialog = await screen.findByRole("dialog");
    await user.click(within(dialog).getByRole("button", { name: "Delete" }));

    await waitFor(() => expect(screen.queryByTitle(name(index))).toBeNull());
    await waitFor(() =>
      expect(document.activeElement).toBe(
        within(
          screen.getByTitle(name(index + 1)).closest("div")!.parentElement!,
        ).getByRole("button", { name: "Rename" }),
      ),
    );
  });

  it("「Tentative only」中の確定で行が外れると、描いていなかった次の行の「改名」へフォーカスが移る", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle(name(0));
    await user.click(screen.getByRole("button", { name: "Tentative only" }));
    const index = await tabPastRange(user);
    expect(document.querySelector(`[data-index="${String(index + 1)}"]`)).toBeNull();

    await user.click(within(wrapperOf(index)).getByRole("button", { name: "Confirm" }));

    await waitFor(() => expect(screen.queryByTitle(name(index))).toBeNull());
    await waitFor(() =>
      expect(document.activeElement).toBe(
        within(
          screen.getByTitle(name(index + 1)).closest("div")!.parentElement!,
        ).getByRole("button", { name: "Rename" }),
      ),
    );
  });
});

describe("TagsPage 並び順と0本の絞り込み", () => {
  /** rowNames は描いている行の名前を一覧の並びで返す。 */
  function rowNames(): string[] {
    return [...document.querySelectorAll("[data-index]")]
      .sort(
        (a, b) =>
          Number((a as HTMLElement).dataset.index) -
          Number((b as HTMLElement).dataset.index),
      )
      .map(
        (row) => row.querySelector("[data-tag-id] [title]")?.getAttribute("title") ?? "",
      );
  }

  async function chooseSort(
    user: ReturnType<typeof userEvent.setup>,
    current: string,
    next: string,
  ) {
    await user.click(screen.getByRole("button", { name: `Sort by: ${current}` }));
    await user.click(await screen.findByRole("menuitemradio", { name: next }));
  }

  beforeEach(() => {
    server.tags = [
      tag({ id: 1, name: "Alpha", videoCount: 5, createdAt: "2025-01-03T00:00:00Z" }),
      tag({ id: 2, name: "Beta", createdAt: "2025-01-05T00:00:00Z" }),
      tag({ id: 3, name: "Cat", videoCount: 5, createdAt: "2025-01-01T00:00:00Z" }),
      tag({ id: 4, name: "Delta", tentative: true, createdAt: "2025-01-02T00:00:00Z" }),
      tag({
        id: 5,
        name: "Echo",
        tentative: true,
        videoCount: 1,
        createdAt: "2025-01-04T00:00:00Z",
      }),
    ];
  });

  it("既定は名前の順で、「Name」のときは向きの切り替えを出さない", async () => {
    install();
    renderPage();
    await screen.findByTitle("Alpha");

    expect(rowNames()).toEqual(["Alpha", "Beta", "Cat", "Delta", "Echo"]);
    expect(screen.getByRole("button", { name: "Sort by: Name" })).toBeDefined();
    expect(screen.queryByRole("button", { name: /Press for/ })).toBeNull();
  });

  it("「Video count」は多い順で最多が先頭・0本が末尾、同じ本数は名前の順（受け入れ条件4）", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("Alpha");

    await chooseSort(user, "Name", "Video count");
    await waitFor(() =>
      expect(rowNames()).toEqual(["Alpha", "Cat", "Echo", "Beta", "Delta"]),
    );

    const toggle = screen.getByRole("button", {
      name: "Descending (most videos first). Press for ascending",
    });
    await user.click(toggle);
    await waitFor(() =>
      expect(rowNames()).toEqual(["Beta", "Delta", "Echo", "Alpha", "Cat"]),
    );
    expect(
      screen.getByRole("button", {
        name: "Ascending (fewest videos first). Press for descending",
      }),
    ).toBeDefined();
  });

  it("「Date created」は新しい順で、作成したタグが先頭に入る（受け入れ条件5）", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("Alpha");

    await chooseSort(user, "Name", "Date created");
    await waitFor(() =>
      expect(rowNames()).toEqual(["Beta", "Echo", "Alpha", "Delta", "Cat"]),
    );

    await user.click(screen.getByRole("button", { name: "New tag" }));
    await user.type(screen.getByRole("textbox", { name: "New tag name" }), "Zulu");
    await user.keyboard("{Enter}");
    await screen.findByTitle("Zulu");
    await waitFor(() => expect(rowNames()[0]).toBe("Zulu"));
  });

  it("並び順は開き直しても残り、壊れた保存は名前の順になる（受け入れ条件6）", async () => {
    const user = userEvent.setup();
    install();
    const first = renderPage();
    await screen.findByTitle("Alpha");
    await chooseSort(user, "Name", "Video count");
    await waitFor(() => expect(rowNames()[0]).toBe("Alpha"));
    await user.click(
      screen.getByRole("button", {
        name: "Descending (most videos first). Press for ascending",
      }),
    );
    first.unmount();

    const second = renderPage();
    await screen.findByTitle("Alpha");
    expect(screen.getByRole("button", { name: "Sort by: Video count" })).toBeDefined();
    expect(rowNames()).toEqual(["Beta", "Delta", "Echo", "Alpha", "Cat"]);
    second.unmount();

    window.localStorage.setItem("vv.tags.v1", "{not json");
    renderPage();
    await screen.findByTitle("Alpha");
    expect(screen.getByRole("button", { name: "Sort by: Name" })).toBeDefined();
    expect(rowNames()).toEqual(["Alpha", "Beta", "Cat", "Delta", "Echo"]);
  });

  it("「Unused only」は0本の行だけを出し、「Tentative only」・検索・並び順と重なる（受け入れ条件7）", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("Alpha");

    const unused = screen.getByRole("button", { name: "Unused only" });
    expect(unused.getAttribute("aria-pressed")).toBe("false");
    await user.click(unused);
    expect(unused.getAttribute("aria-pressed")).toBe("true");
    expect(await screen.findByText("2 of 5 tags")).toBeDefined();
    expect(rowNames()).toEqual(["Beta", "Delta"]);
    for (const name of rowNames()) {
      const row = screen.getByTitle(name).closest("[data-tag-id]")!;
      expect(row.textContent).toContain("0 videos");
    }

    // 並び順と重ねる。
    await chooseSort(user, "Name", "Date created");
    await waitFor(() => expect(rowNames()).toEqual(["Beta", "Delta"]));
    await user.click(
      screen.getByRole("button", {
        name: "Descending (newest first). Press for ascending",
      }),
    );
    await waitFor(() => expect(rowNames()).toEqual(["Delta", "Beta"]));

    // 「Tentative only」と重ねると両方を満たす行だけ。
    await user.click(screen.getByRole("button", { name: "Tentative only" }));
    expect(await screen.findByText("1 of 5 tags")).toBeDefined();
    expect(rowNames()).toEqual(["Delta"]);

    // 検索と重ねる。
    await user.type(screen.getByRole("searchbox", { name: "Search tags" }), "zz");
    expect(await screen.findByText('No unused tentative tags match "zz"')).toBeDefined();
    expect(screen.getByText("0 of 5 tags")).toBeDefined();
  });

  it("0本のタグが無いときは「No unused tags」を出し、「Show all tags」で外して「Unused only」へ戻る", async () => {
    const user = userEvent.setup();
    server.tags = server.tags.map((item) => ({
      ...item,
      videoCount: item.videoCount + 1,
    }));
    install();
    renderPage();
    await screen.findByTitle("Alpha");

    const unused = screen.getByRole("button", { name: "Unused only" });
    await user.click(unused);
    expect(await screen.findByText("No unused tags")).toBeDefined();
    expect(screen.getByText("Every tag is on at least one video.")).toBeDefined();

    await user.click(screen.getByRole("button", { name: "Show all tags" }));
    await waitFor(() => expect(document.activeElement).toBe(unused));
    expect(unused.getAttribute("aria-pressed")).toBe("false");
    expect(screen.getByText("5 tags")).toBeDefined();
  });

  it("「Unused only」と「Tentative only」の両方で一致が無ければ両方を外して「Tentative only」へ戻る", async () => {
    const user = userEvent.setup();
    server.tags = server.tags.map((item) =>
      item.tentative ? { ...item, videoCount: 2 } : item,
    );
    install();
    renderPage();
    await screen.findByTitle("Alpha");

    const tentative = screen.getByRole("button", { name: "Tentative only" });
    await user.click(screen.getByRole("button", { name: "Unused only" }));
    await user.click(tentative);
    expect(await screen.findByText("No unused tentative tags")).toBeDefined();
    expect(screen.queryByText("Every tag is on at least one video.")).toBeNull();

    await user.click(screen.getByRole("button", { name: "Show all tags" }));
    await waitFor(() => expect(document.activeElement).toBe(tentative));
    expect(tentative.getAttribute("aria-pressed")).toBe("false");
    expect(
      screen.getByRole("button", { name: "Unused only" }).getAttribute("aria-pressed"),
    ).toBe("false");
  });

  it("「Unused only」と検索で一致が無ければ、両方を外して検索の入力へ戻る", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("Alpha");

    await user.click(screen.getByRole("button", { name: "Unused only" }));
    const search = screen.getByRole("searchbox", { name: "Search tags" });
    await user.type(search, "Alpha");
    expect(await screen.findByText('No unused tags match "Alpha"')).toBeDefined();

    await user.click(screen.getByRole("button", { name: "Show all tags" }));
    await waitFor(() => expect(document.activeElement).toBe(search));
    expect((search as HTMLInputElement).value).toBe("");
    expect(screen.getByText("5 tags")).toBeDefined();
  });

  it("改名中の行は並び順・絞り込みを変えても残り、打っている途中の名前を失わない", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("Alpha");

    const row = screen.getByTitle("Alpha").closest("[data-tag-id]")!;
    await user.click(within(row as HTMLElement).getByRole("button", { name: "Rename" }));
    const input = await screen.findByRole("textbox", { name: 'New name for "Alpha"' });
    await user.clear(input);
    await user.type(input, "下書き");

    await chooseSort(user, "Name", "Video count");
    await user.click(screen.getByRole("button", { name: "Unused only" }));
    // Alpha は 5 本で「Unused only」に一致しないが、行は本数の順の位置に残る。
    await waitFor(() => expect(screen.getByText("2 of 5 tags")).toBeDefined());
    const still = screen.getByRole("textbox", { name: 'New name for "Alpha"' });
    expect(still).toBe(input);
    expect((still as HTMLInputElement).value).toBe("下書き");
    expect(document.querySelectorAll("[data-index]")).toHaveLength(3);
  });

  it("読み込み中は並び順と「Unused only」が押せない", async () => {
    install();
    holdGetsFrom = 1;
    renderPage();

    expect(
      (screen.getByRole("button", { name: "Unused only" }) as HTMLButtonElement).disabled,
    ).toBe(true);
    expect(
      (screen.getByRole("button", { name: "Sort by: Name" }) as HTMLButtonElement)
        .disabled,
    ).toBe(true);
    expect(
      (screen.getByRole("button", { name: "Sort" }) as HTMLButtonElement).disabled,
    ).toBe(true);
    for (const done of heldGetReleases) done();
  });

  it("狭い幅のまとめから並び順を選べ、Name 以外のときはまとめが効いている形になる", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("Alpha");

    const compact = screen.getByRole("button", { name: "Sort" });
    expect(compact.className).not.toContain("bg-accent-soft");
    await user.click(compact);
    // Name のときは向きを出さない。
    expect(screen.queryByRole("group", { name: "Sort direction" })).toBeNull();
    await user.click(await screen.findByRole("radio", { name: "Video count" }));
    await waitFor(() => expect(rowNames()[0]).toBe("Alpha"));
    await user.click(screen.getByRole("radio", { name: "Fewest videos first" }));
    await waitFor(() => expect(rowNames()[0]).toBe("Beta"));
    expect(screen.getByRole("button", { name: "Sort" }).className).toContain(
      "bg-accent-soft",
    );
  });

  it("疑似ロケールで、並び順と「Unused only」の文言がカタログから出る", async () => {
    enablePseudoLocale();
    const user = userEvent.setup();
    install();
    const { container } = renderPage();
    await screen.findByTitle("Alpha");
    await user.click(screen.getByRole("button", { name: "⟦Unused only⟧" }));
    await screen.findByTitle("Beta");
    expectCatalogTextOnly(container, ["Alpha", "Beta", "Cat", "Delta", "Echo"]);
  });
});
