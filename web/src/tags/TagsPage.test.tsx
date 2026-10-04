import { act, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {
  MemoryRouter,
  type NavigateFunction,
  useLocation,
  useNavigate,
} from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { Tag, TagSort } from "../api/tags";
import { __resetTagsForTest, getTags, subscribeTags } from "../api/tags";
import { enablePseudoLocale, expectCatalogTextOnly } from "../i18n/pseudo";
import { foldForMatch } from "../lib/foldForMatch";
import { compareTagsForSort } from "./tagPageRows";
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

/**
 * pageOf は `GET /api/tags` のページ（specs/036-tag-admin-scale/contracts/screen-api.md §5）を
 * 偽のサーバーの側で作る。条件を全部のタグに掛け、並び順で並べ、カーソル（前のページの
 * 最後の行の並びの値）より後ろから `limit` 件を返す。カーソルは keyset と同じく、
 * 並びの値で「より後ろ」を決める（読み込んだあとで前へ動いた行は返さない）。
 */
function pageOf(tags: readonly Tag[], params: URLSearchParams) {
  const q = foldForMatch(params.get("q") ?? "").trim();
  const sort = (params.get("sort") ?? "name") as TagSort;
  const matched = tags
    .filter((item) => params.get("tentative") !== "true" || item.tentative)
    .filter((item) => params.get("unused") !== "true" || item.videoCount === 0)
    .filter(
      (item) =>
        q === "" ||
        foldForMatch(item.name).includes(q) ||
        item.synonyms.some((synonym) => foldForMatch(synonym).includes(q)),
    )
    .sort((a, b) => compareTagsForSort(a, b, sort));
  const cursor = params.get("cursor");
  const after =
    cursor === null
      ? matched
      : matched.filter(
          (item) =>
            compareTagsForSort(
              item,
              JSON.parse(decodeURIComponent(atob(cursor))) as Tag,
              sort,
            ) > 0,
        );
  const limit = Number(params.get("limit"));
  const items = after.slice(0, limit).map((item) => structuredClone(item));
  const last = items.at(-1);
  return {
    items,
    total: matched.length,
    totalAll: tags.length,
    ...(after.length > limit && last !== undefined
      ? { nextCursor: btoa(encodeURIComponent(JSON.stringify(last))) }
      : {}),
  };
}

/** server はタグの管理経路（GET・POST・PATCH・DELETE /api/tags*）を扱う偽のサーバーである。 */
const server = {
  tags: [] as Tag[],
  nextId: 100,
  /** タグ管理画面のページの GET /api/tags（limit 付き）の回数。 */
  getCalls: 0,
  /** タグ管理画面のページの要求のパラメータ（送った順）。 */
  pageRequests: [] as URLSearchParams[],
  /** 共有の保持の全件の GET /api/tags（limit 無し）の回数。 */
  fullGetCalls: 0,
  /** true にすると、共有の保持の全件の GET /api/tags（limit 無し）を 500 で失敗させる。 */
  failFullGets: false,
  /** 設定すると、ページの GET /api/tags の limit の代わりにこの件数を返す。 */
  pageLimitOverride: null as number | null,
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
  /** true にすると、以後の cursor 付きの GET /api/tags/rejected-names（続き）を 500 で失敗させる。 */
  failRejectedMoreGets: false,
  /** GET /api/tags/rejected-names が受けたクエリ。 */
  rejectedGetRequests: [] as URLSearchParams[],
  /** 確定・却下・却下した名前の取り外しの要求の回数。 */
  confirmCalls: 0,
  /** POST /api/tags/batch・/api/tags/impact が受けた本文。 */
  batchCalls: [] as { action: string; ids: number[] }[],
  impactCalls: [] as { action: string; ids: number[] }[],
  /** 設定すると、POST /api/tags/impact の videoCount をこの値にする（重複を除いた数の代わり）。 */
  impactVideoCount: null as number | null,
  /** true にすると、以後の POST /api/tags/impact を 500 で失敗させる。 */
  failImpact: false,
  /** true にすると、次の POST /api/tags/batch を 500 で失敗させる。 */
  failNextBatch: false,
};

/**
 * holdRejectedGets を true にした間の GET /api/tags/rejected-names は、release を
 * 呼ぶまで応答しない。応答の中身は要求を受けた時点の `server.rejectedNames` を
 * 写し取って持つ（取り外しの前に始まった取り直しの古い応答を再現する）。
 */
let holdRejectedGets = false;
const rejectedGetReleases: (() => void)[] = [];

/**
 * holdRejectedDeletes を true にした間の DELETE /api/tags/rejected-names は、release を
 * 呼ぶまでサーバーの並びを変えず、応答もしない。
 */
let holdRejectedDeletes = false;
const rejectedDeleteReleases: (() => void)[] = [];

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

    if (path === "/api/tags" && method === "GET" && !url.searchParams.has("limit")) {
      // 共有の保持（候補・絞り込みの確かめ）の全件。タグ管理画面は送らない。
      server.fullGetCalls += 1;
      if (server.failFullGets) {
        return Promise.resolve(
          jsonResponse({ code: "internal", message: "failed" }, 500),
        );
      }
      const items = server.tags.map((item) => structuredClone(item));
      return Promise.resolve(
        jsonResponse({ items, total: items.length, totalAll: items.length }),
      );
    }

    if (path === "/api/tags" && method === "GET") {
      server.getCalls += 1;
      server.pageRequests.push(url.searchParams);
      const failed = () => jsonResponse({ code: "internal", message: "failed" }, 500);
      const fail = server.failAllGets || server.failNextGet;
      server.failNextGet = false;
      // 応答の中身は要求を受けた時点のサーバーの状態で作る。
      const params = new URLSearchParams(url.searchParams);
      if (server.pageLimitOverride !== null) {
        params.set("limit", String(server.pageLimitOverride));
      }
      const body = fail ? null : pageOf(server.tags, params);
      const respond = () => (body === null ? failed() : jsonResponse(body));
      if (holdGetsFrom !== null && server.getCalls >= holdGetsFrom) {
        return new Promise((resolve, reject) => {
          init?.signal?.addEventListener("abort", () => {
            reject(new DOMException("aborted", "AbortError"));
          });
          heldGetReleases.push(() => resolve(respond()));
        });
      }
      return Promise.resolve(respond());
    }

    if (path === "/api/tags/rejected-names" && method === "GET") {
      if (server.failRejectedGets) {
        return Promise.resolve(
          jsonResponse({ code: "internal", message: "failed" }, 500),
        );
      }
      server.rejectedGetRequests.push(url.searchParams);
      const cursor = url.searchParams.get("cursor");
      if (cursor !== null && server.failRejectedMoreGets) {
        return Promise.resolve(
          jsonResponse({ code: "internal", message: "failed" }, 500),
        );
      }
      // 実際のサーバーと同じく、cursor は前のページの最後の名前の次から、limit 件ずつ返す
      // （名前は昇順で持つ）。
      const snapshot = [...server.rejectedNames];
      const limit = Number(url.searchParams.get("limit") ?? "100");
      const start = cursor === null ? 0 : snapshot.findIndex((name) => name > cursor);
      const from = start < 0 ? snapshot.length : start;
      const items = snapshot.slice(from, from + limit);
      const page = {
        items,
        total: snapshot.length,
        ...(from + limit < snapshot.length
          ? { nextCursor: items[items.length - 1] }
          : {}),
      };
      if (holdRejectedGets) {
        return new Promise((resolve) => {
          rejectedGetReleases.push(() => resolve(jsonResponse(page)));
        });
      }
      return Promise.resolve(jsonResponse(page));
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

    if (path === "/api/tags/batch" && method === "POST") {
      const body = JSON.parse(String(init?.body)) as { action: string; ids: number[] };
      server.batchCalls.push(body);
      if (server.failNextBatch) {
        server.failNextBatch = false;
        return Promise.resolve(
          jsonResponse({ code: "internal", message: "failed" }, 500),
        );
      }
      const appliedIds: number[] = [];
      const notFoundIds: number[] = [];
      const notApplicableIds: number[] = [];
      for (const id of body.ids) {
        const found = server.tags.find((t) => t.id === id);
        if (found === undefined) notFoundIds.push(id);
        else if (found.tentative === (body.action === "delete"))
          notApplicableIds.push(id);
        else appliedIds.push(id);
      }
      const applied = new Set(appliedIds);
      if (body.action === "confirm") {
        for (const item of server.tags) if (applied.has(item.id)) item.tentative = false;
      } else {
        if (body.action === "reject") {
          server.rejectedNames = [
            ...server.rejectedNames,
            ...server.tags.filter((t) => applied.has(t.id)).map((t) => t.name),
          ].sort();
        }
        server.tags = server.tags.filter((t) => !applied.has(t.id));
      }
      return maybeHold(() => jsonResponse({ appliedIds, notFoundIds, notApplicableIds }));
    }

    if (path === "/api/tags/impact" && method === "POST") {
      const body = JSON.parse(String(init?.body)) as { action: string; ids: number[] };
      server.impactCalls.push(body);
      if (server.failImpact) {
        return Promise.resolve(
          jsonResponse({ code: "internal", message: "failed" }, 500),
        );
      }
      const counted = server.tags.filter(
        (t) =>
          body.ids.includes(t.id) &&
          (body.action === "merge" || t.tentative === (body.action === "reject")),
      );
      return Promise.resolve(
        jsonResponse({
          tagCount: counted.length,
          videoCount:
            server.impactVideoCount ?? counted.reduce((sum, t) => sum + t.videoCount, 0),
        }),
      );
    }

    if (path === "/api/tags/rejected-names" && method === "DELETE") {
      const name = url.searchParams.get("name") ?? "";
      const respond = () => {
        server.rejectedNames = server.rejectedNames.filter((item) => item !== name);
        return jsonResponse(null, 204);
      };
      if (holdRejectedDeletes) {
        return new Promise((resolve) => {
          rejectedDeleteReleases.push(() => resolve(respond()));
        });
      }
      return Promise.resolve(respond());
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

/** latestSearch は `LocationProbe` が最後に見た URL のクエリである。 */
let latestSearch = "";

/** historyNavigate はブラウザの戻る・進む（`navigate(-1)` など）である。 */
let historyNavigate: NavigateFunction | null = null;

/** LocationProbe は今の URL のクエリを `latestSearch` に写し、履歴を動かす口を残す。 */
function LocationProbe() {
  latestSearch = useLocation().search;
  historyNavigate = useNavigate();
  return null;
}

/** historyBack はブラウザの「戻る」を押す。 */
async function historyBack() {
  await act(async () => {
    void historyNavigate!(-1);
    await Promise.resolve();
  });
}

/**
 * renderPage はタグ管理画面を描く。`url` で開く URL（検索語・絞り込み・並び順・タブの
 * クエリ）を、`topBar` で共通トップバーの差し込み先（`TopBarPortal`）を置ける。
 */
function renderPage({
  url = "/tags",
  topBar = false,
}: { url?: string; topBar?: boolean } = {}) {
  return render(
    <MemoryRouter initialEntries={[url]}>
      <TooltipProvider>
        <ToastProvider>
          {topBar && (
            <header data-testid="topbar">
              <div id="topbar-library-tools" />
            </header>
          )}
          <TagsPage />
          <LocationProbe />
        </ToastProvider>
      </TooltipProvider>
    </MemoryRouter>,
  );
}

/**
 * filterButton はトップバーの「Filter」である。効いている間の読み上げ名は
 * 「Filter (1 applied)」になる。
 */
function filterButton(): HTMLElement {
  return screen.getByRole("button", { name: /^Filter/ });
}

type FilterName = "Tentative only" | "Unused only";

/**
 * toggleFilter は「Filter」の吹き出しを開いて、その絞り込みのチェックを押し、Esc で
 * 吹き出しを閉じる（フォーカスは「Filter」へ戻る）。
 */
async function toggleFilter(user: ReturnType<typeof userEvent.setup>, name: FilterName) {
  await user.click(filterButton());
  await user.click(await screen.findByRole("checkbox", { name: new RegExp(`^${name}`) }));
  await user.keyboard("{Escape}");
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
}

/** filterChecked は吹き出しを開いて、その絞り込みのチェックの状態を読み、閉じる。 */
async function filterChecked(
  user: ReturnType<typeof userEvent.setup>,
  name: FilterName,
): Promise<boolean> {
  await user.click(filterButton());
  const box = await screen.findByRole("checkbox", { name: new RegExp(`^${name}`) });
  const checked = box.getAttribute("aria-checked") === "true";
  await user.keyboard("{Escape}");
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  return checked;
}

/** rejectedTab は見出しの下の「Rejected names」のタブ（件数を添える）である。 */
function rejectedTab(): HTMLElement {
  return screen.getByRole("tab", { name: /Rejected names/ });
}

/** tagsTab は「Tags」のタブである。 */
function tagsTab(): HTMLElement {
  return screen.getByRole("tab", { name: /^Tags/ });
}

/** openRejectedTab は「Rejected names」のタブへ移り、却下した名前の並びを返す。 */
async function openRejectedTab(user: ReturnType<typeof userEvent.setup>) {
  await user.click(rejectedTab());
  const panel = await screen.findByRole("tabpanel", { name: /Rejected names/ });
  return within(panel).findByRole("table", { name: "Rejected names" });
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
  server.pageRequests = [];
  server.fullGetCalls = 0;
  server.pageLimitOverride = null;
  server.failFullGets = false;
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
  server.failRejectedMoreGets = false;
  server.rejectedGetRequests = [];
  server.confirmCalls = 0;
  server.batchCalls = [];
  server.impactCalls = [];
  server.impactVideoCount = null;
  server.failImpact = false;
  server.failNextBatch = false;
  holdConfirms = false;
  confirmReleases.length = 0;
  holdRejectedGets = false;
  rejectedGetReleases.length = 0;
  holdRejectedDeletes = false;
  rejectedDeleteReleases.length = 0;
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
    const dramaRow = screen.getByTitle("Drama").closest<HTMLElement>("[data-tag-id]")!;
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

    const row = screen.getByTitle("旅行").closest<HTMLElement>("[data-tag-id]")!;
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

    const row = screen.getByTitle("旅行").closest<HTMLElement>("[data-tag-id]")!;
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

    const row = screen.getByTitle("旅行").closest<HTMLElement>("[data-tag-id]")!;
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

    const row = screen.getByTitle("Anime").closest<HTMLElement>("[data-tag-id]")!;
    await user.click(within(row).getByRole("button", { name: "More actions" }));
    await user.click(await screen.findByRole("menuitem", { name: "Delete…" }));

    const dialog = await screen.findByRole("alertdialog", { name: 'Delete "Anime"' });
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
        within(
          screen.getByTitle("Drama").closest<HTMLElement>("[data-tag-id]")!,
        ).getByRole("button", { name: "Rename" }),
      ),
    );
  });

  it("0本のタグの削除確認は「どの動画にも付いていません」と出す", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("Drama");

    const row = screen.getByTitle("Drama").closest<HTMLElement>("[data-tag-id]")!;
    await user.click(within(row).getByRole("button", { name: "More actions" }));
    await user.click(await screen.findByRole("menuitem", { name: "Delete…" }));

    const dialog = await screen.findByRole("alertdialog", { name: 'Delete "Drama"' });
    expect(within(dialog).getByText("This tag isn't on any videos.")).toBeDefined();
  });

  it("削除の確認でEscを押すと何も変わらず、フォーカスがその行の「その他の操作」へ戻る（B2）", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("Drama");

    const row = screen.getByTitle("Drama").closest<HTMLElement>("[data-tag-id]")!;
    const menuButton = within(row).getByRole("button", { name: "More actions" });
    await user.click(menuButton);
    await user.click(await screen.findByRole("menuitem", { name: "Delete…" }));
    await screen.findByRole("alertdialog", { name: 'Delete "Drama"' });

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

    const row = screen.getByTitle("Drama").closest<HTMLElement>("[data-tag-id]")!;
    const menuButton = within(row).getByRole("button", { name: "More actions" });
    await user.click(menuButton);
    await user.click(await screen.findByRole("menuitem", { name: "Delete…" }));
    const dialog = await screen.findByRole("alertdialog", { name: 'Delete "Drama"' });

    await user.click(within(dialog).getByRole("button", { name: "Cancel" }));

    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    await waitFor(() => expect(document.activeElement).toBe(menuButton));
  });

  it("別のタブで消したタグの削除を試みると、もう無いことが伝わり次の行の改名へフォーカスが移る（B2）", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("Anime");

    const row = screen.getByTitle("Anime").closest<HTMLElement>("[data-tag-id]")!;
    await user.click(within(row).getByRole("button", { name: "More actions" }));
    await user.click(await screen.findByRole("menuitem", { name: "Delete…" }));
    const dialog = await screen.findByRole("alertdialog", { name: 'Delete "Anime"' });

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
        within(
          screen.getByTitle("Drama").closest<HTMLElement>("[data-tag-id]")!,
        ).getByRole("button", { name: "Rename" }),
      ),
    );
  });

  it("キーボードだけで検索の入力に届く（見出しの「New tag」とタブの次のTab）", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("旅行");

    await user.tab();
    expect(document.activeElement).toBe(screen.getByRole("button", { name: "New tag" }));
    await user.tab();
    expect(document.activeElement).toBe(screen.getByRole("tab", { name: /^Tags/ }));
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
    const row = screen.getByTitle("旅行").closest<HTMLElement>("[data-tag-id]")!;
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

    const row = screen.getByTitle("旅行").closest<HTMLElement>("[data-tag-id]")!;
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

    const row = screen.getByTitle("Anime").closest<HTMLElement>("[data-tag-id]")!;
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

    const row = screen.getByTitle("旅行").closest<HTMLElement>("[data-tag-id]")!;
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

    const row = screen.getByTitle("旅行").closest<HTMLElement>("[data-tag-id]")!;
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

    const dramaRow = screen.getByTitle("Drama").closest<HTMLElement>("[data-tag-id]")!;
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

  it("作成の後に一覧を取り直さず、共有の保持の購読者が無ければ全件の GET も送らない（R-12）", async () => {
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

    // 作った 1 件は読み込んだ行の中に置き、ページも全件も取り直さない。
    await new Promise((resolve) => setTimeout(resolve, 50));
    expect(server.getCalls).toBe(1);
    expect(server.fullGetCalls).toBe(0);
  });

  it("共有の保持の購読者がいれば、操作のあとに全件の GET が送られる（R-12）", async () => {
    const user = userEvent.setup();
    install();
    // 候補や絞り込みの確かめが共有の保持を購読している（別の画面の部品など）。
    const unsubscribe = subscribeTags(() => undefined);
    try {
      renderPage();
      await screen.findByTitle("旅行");
      expect(server.fullGetCalls).toBe(0);

      const row = screen.getByTitle("Drama").closest<HTMLElement>("[data-tag-id]")!;
      await user.click(within(row).getByRole("button", { name: "More actions" }));
      await user.click(await screen.findByRole("menuitem", { name: "Delete…" }));
      await user.click(
        within(await screen.findByRole("alertdialog")).getByRole("button", {
          name: "Delete",
        }),
      );
      await waitFor(() => expect(screen.queryByTitle("Drama")).toBeNull());
      await waitFor(() => expect(server.fullGetCalls).toBe(1));
      expect(server.getCalls).toBe(1);
    } finally {
      unsubscribe();
    }
  });

  it("購読者が無ければ、操作のあとに共有の保持を捨てて次の getTags が取り直す（R-12）", async () => {
    const user = userEvent.setup();
    install();
    await getTags();
    expect(server.fullGetCalls).toBe(1);
    renderPage();
    await screen.findByTitle("旅行");

    const row = screen.getByTitle("Drama").closest<HTMLElement>("[data-tag-id]")!;
    await user.click(within(row).getByRole("button", { name: "More actions" }));
    await user.click(await screen.findByRole("menuitem", { name: "Delete…" }));
    await user.click(
      within(await screen.findByRole("alertdialog")).getByRole("button", {
        name: "Delete",
      }),
    );
    await waitFor(() => expect(screen.queryByTitle("Drama")).toBeNull());
    await new Promise((resolve) => setTimeout(resolve, 20));
    expect(server.fullGetCalls).toBe(1);

    const refreshed = await getTags();
    expect(server.fullGetCalls).toBe(2);
    expect(refreshed.map((item) => item.name)).not.toContain("Drama");
  });

  it("tag_not_foundの取り直しに失敗しても、一覧を空白にせず今の一覧を残す（N6）", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("旅行");

    const row = screen.getByTitle("旅行").closest<HTMLElement>("[data-tag-id]")!;
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

    const row = screen.getByTitle("旅行").closest<HTMLElement>("[data-tag-id]")!;
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

    const row = screen.getByTitle("旅行").closest<HTMLElement>("[data-tag-id]")!;
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

    const row = screen.getByTitle("旅行").closest<HTMLElement>("[data-tag-id]")!;
    await user.click(within(row).getByRole("button", { name: "Rename" }));
    const input = await screen.findByRole("textbox", { name: 'New name for "旅行"' });
    await user.clear(input);
    await user.type(input, "旅行2024");

    holdNextMutation = true;
    await user.keyboard("{Enter}");
    await waitFor(() => expect(input.getAttribute("aria-busy")).toBe("true"));

    const animeRow = screen.getByTitle("Anime").closest<HTMLElement>("[data-tag-id]")!;
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

    const row = screen.getByTitle("旅行").closest<HTMLElement>("[data-tag-id]")!;
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

    const row = screen.getByTitle("旅行").closest<HTMLElement>("[data-tag-id]")!;
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

    const row = screen.getByTitle("旅行").closest<HTMLElement>("[data-tag-id]")!;
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

    const row = screen.getByTitle("旅行").closest<HTMLElement>("[data-tag-id]")!;
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

    const row = screen.getByTitle("旅行").closest<HTMLElement>("[data-tag-id]")!;
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

    const row = screen.getByTitle("旅行").closest<HTMLElement>("[data-tag-id]")!;
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

    const row = screen.getByTitle("旅行").closest<HTMLElement>("[data-tag-id]")!;
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

  it("統合先の候補は窓の本文に開いたままで、入力の中の Esc は窓を閉じる（B1）", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("旅行");

    const row = screen.getByTitle("旅行").closest<HTMLElement>("[data-tag-id]")!;
    await user.click(within(row).getByRole("button", { name: "More actions" }));
    await user.click(
      await screen.findByRole("menuitem", { name: "Merge into another tag…" }),
    );
    const dialog = await screen.findByRole("dialog", { name: 'Merge "旅行"' });

    const combo = within(dialog).getByRole("combobox", { name: "Tag to merge into" });
    await user.click(combo);
    await user.type(combo, "A");
    expect(combo.getAttribute("aria-expanded")).toBe("true");

    // 候補の一覧は窓の本文の一部で、閉じる段は無い。Esc で窓が閉じる。
    await user.keyboard("{Escape}");
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  });

  it("IME変換中のEscは窓を閉じない（B2）", async () => {
    install();
    renderPage();
    await screen.findByTitle("旅行");

    const user = userEvent.setup();
    const row = screen.getByTitle("旅行").closest<HTMLElement>("[data-tag-id]")!;
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

    const row = screen.getByTitle("旅行").closest<HTMLElement>("[data-tag-id]")!;
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

    // 送っている間は「Cancel」も押せない。
    expect(
      within(dialog).getByRole("button", { name: "Cancel" }).hasAttribute("disabled"),
    ).toBe(true);
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

    const row = screen.getByTitle("旅行").closest<HTMLElement>("[data-tag-id]")!;
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

    const row = screen.getByTitle("Anime").closest<HTMLElement>("[data-tag-id]")!;
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

    const row = screen.getByTitle("Drama").closest<HTMLElement>("[data-tag-id]")!;
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

    const row = screen.getByTitle("Drama").closest<HTMLElement>("[data-tag-id]")!;
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

    const row = screen.getByTitle("Anime").closest<HTMLElement>("[data-tag-id]")!;
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

    const row = screen.getByTitle("Anime").closest<HTMLElement>("[data-tag-id]")!;
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

    const row = screen.getByTitle("Anime").closest<HTMLElement>("[data-tag-id]")!;
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

    const row = screen.getByTitle("Anime").closest<HTMLElement>("[data-tag-id]")!;
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

    const row = screen.getByTitle("Anime").closest<HTMLElement>("[data-tag-id]")!;
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

    const row = screen.getByTitle("Drama").closest<HTMLElement>("[data-tag-id]")!;
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

    const row = screen.getByTitle("Anime").closest<HTMLElement>("[data-tag-id]")!;
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

    const row = screen.getByTitle("Anime").closest<HTMLElement>("[data-tag-id]")!;
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

    const row = screen.getByTitle("Anime").closest<HTMLElement>("[data-tag-id]")!;
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

    const row = screen.getByTitle("Drama").closest<HTMLElement>("[data-tag-id]")!;
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

    const row = screen.getByTitle("Anime").closest<HTMLElement>("[data-tag-id]")!;
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

    const row = screen.getByTitle("Drama").closest<HTMLElement>("[data-tag-id]")!;
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

    const row = screen.getByTitle("Anime").closest<HTMLElement>("[data-tag-id]")!;
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
    return screen.getByTitle(name).closest<HTMLElement>("[data-tag-id]")!;
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
    await screen.findByRole("alertdialog");
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
    await screen.findAllByRole("status");
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
    const dialog = await screen.findByRole("alertdialog", { name: 'Delete "旅行"' });
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
    return screen.getByTitle(name).closest<HTMLElement>("[data-tag-id]")!;
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
    // 確定した行の操作は今と同じ3つ（マウスの端末）と、タッチ・狭い幅でそれを
    // まとめる「Actions」（CSS でどちらか一方だけを描く）。
    expect(
      within(gamma)
        .getAllByRole("button")
        .map((button) => button.getAttribute("aria-label")),
    ).toEqual(["Actions", "Rename", "Synonyms", "More actions"]);
  });

  it("「Tentative only」で仮のタグだけが並び、件数は全タグを分母にする（受け入れ条件6）", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("Gamma");

    expect(await filterChecked(user, "Tentative only")).toBe(false);
    expect(filterButton().getAttribute("aria-label")).toBe("Filter");
    await toggleFilter(user, "Tentative only");

    expect(await filterChecked(user, "Tentative only")).toBe(true);
    expect(filterButton().getAttribute("aria-label")).toBe("Filter (1 applied)");
    expect(await screen.findByText("3 of 4 tags")).toBeDefined();
    expect(screen.queryByTitle("Gamma")).toBeNull();
    expect(screen.getByTitle("Alpha")).toBeDefined();

    // 検索と重ねられる。
    await user.type(screen.getByRole("searchbox", { name: "Search tags" }), "be");
    expect(await screen.findByText("1 of 4 tags")).toBeDefined();
    expect(screen.getByTitle("Beta")).toBeDefined();
    expect(screen.queryByTitle("Alpha")).toBeNull();

    await toggleFilter(user, "Tentative only");
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
    await toggleFilter(user, "Tentative only");

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

    const entry = rejectedTab();
    await waitFor(() => expect(entry.textContent).toContain("0"));

    await user.click(
      within(rowOf("Alpha")).getByRole("button", { name: "More actions" }),
    );
    await user.click(await screen.findByRole("menuitem", { name: "Reject…" }));
    const dialog = await screen.findByRole("alertdialog", { name: 'Reject "Alpha"' });
    expect(
      within(dialog).getByText(
        'This tag will be removed from 2 videos, and automatic tagging won\'t create "Alpha" again. You can allow the name again from Rejected names.',
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
    await waitFor(() => expect(entry.textContent).toContain("1"));

    const list = await openRejectedTab(user);
    expect(within(list).getByTitle("Alpha")).toBeDefined();
  });

  it("0本の仮のタグの却下の確認は、どの動画にも付いていないと出す", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("Beta");

    await user.click(within(rowOf("Beta")).getByRole("button", { name: "More actions" }));
    await user.click(await screen.findByRole("menuitem", { name: "Reject…" }));
    const dialog = await screen.findByRole("alertdialog", { name: 'Reject "Beta"' });
    expect(
      within(dialog).getByText(
        "This tag isn't on any videos. Automatic tagging won't create \"Beta\" again. You can allow the name again from Rejected names.",
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
    const dialog = await screen.findByRole("alertdialog", { name: 'Reject "Alpha"' });

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
    const dialog = await screen.findByRole("alertdialog", { name: 'Reject "Alpha"' });
    server.nextError = { status: 500, body: { code: "internal", message: "x" } };
    await user.click(within(dialog).getByRole("button", { name: "Reject" }));

    expect(await within(dialog).findByRole("alert")).toBeDefined();
    expect(screen.getByRole("alertdialog", { name: 'Reject "Alpha"' })).toBeDefined();
  });

  it("却下した名前を「Allow again」で一覧から外せる。最後の1つならタブへフォーカスが移る（受け入れ条件13）", async () => {
    const user = userEvent.setup();
    server.rejectedNames = ["Old", "Stale"];
    install();
    renderPage();
    await screen.findByTitle("Alpha");

    const entry = rejectedTab();
    await waitFor(() => expect(entry.textContent).toContain("2"));
    const list = await openRejectedTab(user);
    expect(entry.getAttribute("aria-selected")).toBe("true");
    expect(
      screen.getByText(
        "Automatic tagging won't create these names. Allow a name again to let it be created.",
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
    await waitFor(() => expect(document.activeElement).toBe(rejectedTab()));
    expect(rejectedTab().textContent).toContain("0");
  });

  it("却下した名前は見出しの下のタブで、選ぶと本文に一覧と取り外しが出る（受け入れ条件12）", async () => {
    const user = userEvent.setup();
    server.rejectedNames = ["Old"];
    install();
    renderPage();
    await screen.findByTitle("Alpha");

    const entry = rejectedTab();
    const firstRow = screen.getByTitle("Alpha").closest("[data-tag-id]")!;
    expect(
      entry.compareDocumentPosition(firstRow) & Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();
    // タブは見出しと同じ帯の中にある。窓は開かない。
    expect(entry.closest(".sticky")).toBe(
      screen.getByRole("heading", { name: "Tags" }).closest(".sticky"),
    );
    expect(entry.getAttribute("aria-haspopup")).toBeNull();
    expect(tagsTab().getAttribute("aria-selected")).toBe("true");
    await waitFor(() => expect(entry.textContent).toContain("1"));

    const list = await openRejectedTab(user);
    expect(screen.queryByRole("dialog")).toBeNull();
    // タグの行は「Tags」のタブに戻るまで描かない。
    expect(
      screen.queryByRole("link", { name: "Open the library filtered by Alpha" }),
    ).toBeNull();
    expect(
      screen.getByText(
        "Automatic tagging won't create these names. Allow a name again to let it be created.",
      ),
    ).toBeDefined();
    await user.click(within(list).getByRole("button", { name: 'Allow "Old" again' }));
    expect(await screen.findByText("No rejected names")).toBeDefined();
    expect(server.rejectedNames).toEqual([]);

    await user.click(tagsTab());
    expect(await screen.findByTitle("Alpha")).toBeDefined();
  });

  it("タグの一覧を取る前もタブは出し、件数は届いてから添える。タグが無いときも出す", async () => {
    server.tags = [];
    install();
    renderPage();
    expect(rejectedTab()).toBeDefined();
    expect(await screen.findByText("No tags yet")).toBeDefined();
    await waitFor(() => expect(rejectedTab().textContent).toContain("0"));
    expect(tagsTab().textContent).toContain("0");
  });

  it("却下した名前を取れなかったら理由を出し、再試行できる", async () => {
    const user = userEvent.setup();
    server.failRejectedGets = true;
    server.rejectedNames = ["Old"];
    install();
    renderPage();
    await screen.findByTitle("Alpha");

    await user.click(rejectedTab());
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

    // 却下で取り直しが始まり、その応答は取り外しの前の並びを持ったまま止まる。
    holdRejectedGets = true;
    await user.click(within(rowOf("Beta")).getByRole("button", { name: "More actions" }));
    await user.click(await screen.findByRole("menuitem", { name: "Reject…" }));
    await user.click(await screen.findByRole("button", { name: "Reject" }));
    await waitFor(() => expect(screen.queryByTitle("Beta")).toBeNull());
    expect(rejectedGetReleases).toHaveLength(1);
    holdRejectedGets = false;

    const list = await openRejectedTab(user);

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
    await user.click(rejectedTab());
    expect(
      await screen.findByRole("button", { name: 'Allow "Old" again' }),
    ).toBeDefined();
    await user.click(tagsTab());
    await screen.findByTitle("Beta");

    server.failRejectedGets = true;
    await user.click(within(rowOf("Beta")).getByRole("button", { name: "More actions" }));
    await user.click(await screen.findByRole("menuitem", { name: "Reject…" }));
    await user.click(await screen.findByRole("button", { name: "Reject" }));
    await waitFor(() => expect(screen.queryByTitle("Beta")).toBeNull());

    await user.click(rejectedTab());
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
    await toggleFilter(user, "Tentative only");

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
    await toggleFilter(user, "Tentative only");

    holdConfirms = true;
    await user.click(within(rowOf("Alpha")).getByRole("button", { name: "Confirm" }));
    await toggleFilter(user, "Tentative only");
    expect(filterButton().getAttribute("aria-label")).toBe("Filter");

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
    await toggleFilter(user, "Tentative only");

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

    const entry = rejectedTab();
    await waitFor(() => expect(entry.textContent).toContain("1"));

    await user.click(screen.getByRole("button", { name: "New tag" }));
    await user.type(screen.getByRole("textbox", { name: "New tag name" }), "Old");
    await user.keyboard("{Enter}");
    await screen.findByTitle("Old");

    await waitFor(() => expect(entry.textContent).toContain("0"));
    await user.click(entry);
    expect(await screen.findByText("No rejected names")).toBeDefined();
    expect(screen.queryByRole("button", { name: 'Allow "Old" again' })).toBeNull();
  });

  it("「Tentative only」中の仮のタグどうしの統合は、「新しいタグ」へ落ちず次の行へフォーカスが移る", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("Alpha");
    await toggleFilter(user, "Tentative only");

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

  it("「Tentative only」中に唯一のタグを却下すると、「Filter」が押せるまま、そこにフォーカスがある", async () => {
    const user = userEvent.setup();
    server.tags = [tag({ id: 1, name: "Alpha", tentative: true })];
    install();
    renderPage();
    await screen.findByTitle("Alpha");
    await toggleFilter(user, "Tentative only");
    const filter = filterButton();

    await user.click(
      within(rowOf("Alpha")).getByRole("button", { name: "More actions" }),
    );
    await user.click(await screen.findByRole("menuitem", { name: "Reject…" }));
    const dialog = await screen.findByRole("alertdialog", { name: 'Reject "Alpha"' });
    await user.click(within(dialog).getByRole("button", { name: "Reject" }));

    expect(await screen.findByText("No tentative tags")).toBeDefined();
    expect(filter.hasAttribute("disabled")).toBe(false);
    await waitFor(() => expect(document.activeElement).toBe(filter));

    // 「Show all tags」で外すとタグが 0 なので、フォーカスは「新しいタグ」へ。
    await user.click(screen.getByRole("button", { name: "Show all tags" }));
    expect(await screen.findByText("No tags yet")).toBeDefined();
    expect(filter.hasAttribute("disabled")).toBe(true);
    await waitFor(() =>
      expect(document.activeElement).toBe(
        screen.getAllByRole("button", { name: "New tag" })[0],
      ),
    );
  });

  it("「Tentative only」中に唯一の仮のタグへシノニムを足すと、入力にフォーカスが残り、閉じると「Filter」へ移る", async () => {
    const user = userEvent.setup();
    server.tags = [
      tag({ id: 1, name: "Alpha", tentative: true }),
      tag({ id: 4, name: "Gamma" }),
    ];
    install();
    renderPage();
    await screen.findByTitle("Alpha");
    await toggleFilter(user, "Tentative only");
    const toggle = filterButton();

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
    await toggleFilter(user, "Tentative only");
    const search = screen.getByRole("searchbox", { name: "Search tags" });
    await user.type(search, "Gamma");

    expect(await screen.findByText('No tentative tags match "Gamma"')).toBeDefined();
    await user.click(screen.getByRole("button", { name: "Show all tags" }));
    await waitFor(() => expect(document.activeElement).toBe(search));
    expect(filterButton().getAttribute("aria-label")).toBe("Filter");
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

    await user.click(screen.getByRole("tab", { name: /Rejected names/ }));
    await screen.findByRole("table", { name: /Rejected names/ });
    expectCatalogTextOnly(document.body, userData);
    await user.click(screen.getByRole("tab", { name: /Tags/ }));
    await screen.findByTitle("Alpha");

    await user.click(screen.getByRole("button", { name: /Filter/ }));
    expectCatalogTextOnly(document.body, userData);
    await user.click(await screen.findByRole("checkbox", { name: /Tentative only/ }));
    await user.keyboard("{Escape}");
    await waitFor(() => expect(screen.queryByTitle("Gamma")).toBeNull());
    expectCatalogTextOnly(document.body, userData);

    await user.click(
      within(rowOf("Alpha")).getByRole("button", { name: /More actions/ }),
    );
    await screen.findByRole("menu");
    expectCatalogTextOnly(document.body, userData);
    await user.click(screen.getByRole("menuitem", { name: /Reject/ }));
    await screen.findByRole("alertdialog");
    expectCatalogTextOnly(document.body, userData);
  });
});

describe("TagsPage 見えている行だけ描く", () => {
  /** jsdom には表示域の高さと要素の高さが無いので、試験用の高さを置く。 */
  const rowHeight = 40;
  const viewportHeight = 200;
  const tagCount = 60;
  const bandHeight = 50;
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
    // 表示域の上に重なる上部バーの高さ（tokens.css の --spacing-navbar）。行へ
    // スクロールするときにこの分を差し引く（ui-design.md「Band」）。
    document.documentElement.style.setProperty(
      "--spacing-navbar",
      `${String(bandHeight)}px`,
    );
  });

  afterEach(() => {
    document.documentElement.style.removeProperty("--spacing-navbar");
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

    // 描いた次の行のチェックへ移る（ui-design.md「Keyboard across virtualized rows」）。
    const next = await tabPastRange(user);
    expect(document.activeElement).toBe(
      screen.getByRole("checkbox", { name: `Select "${name(next)}"` }),
    );

    // その行の最後の操作から、まだ描いていない次の行へ。
    within(wrapperOf(next)).getByRole("button", { name: "More actions" }).focus();
    await user.tab();
    expect(document.activeElement).toBe(
      screen.getByRole("checkbox", { name: `Select "${name(next + 1)}"` }),
    );
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

  it("並び順で改名中の行が描いている範囲の外へ移っても、その行を描き続け打っている途中の名前を失わない", async () => {
    const user = userEvent.setup();
    // 先頭の行だけ 0 本にし、本数の多い順で末尾（描いている範囲の外）へ移す。
    server.tags = server.tags.map((item, index) => ({
      ...item,
      videoCount: index === 0 ? 0 : 1,
    }));
    install();
    renderPage();
    await screen.findByTitle(name(0));

    const row = screen.getByTitle(name(0)).closest("[data-tag-id]")!;
    await user.click(within(row as HTMLElement).getByRole("button", { name: "Rename" }));
    const input = await screen.findByRole("textbox", {
      name: `New name for "${name(0)}"`,
    });
    await user.clear(input);
    await user.type(input, "下書き");

    await user.click(screen.getByRole("button", { name: "Sort by: Name" }));
    await user.click(await screen.findByRole("menuitemradio", { name: "Video count" }));
    await waitFor(() => expect(screen.queryByTitle(name(1))).not.toBeNull());
    await waitFor(() => expect(wrapperOf(tagCount - 1)).not.toBeNull());
    await new Promise((resolve) => setTimeout(resolve, 0));

    // 改名中の行は全件の末尾にあり、表示域の外でも描いたままで、値も入力も同じ。
    expect(drawnIndexes()).toContain(tagCount - 1);
    expect(drawnIndexes()).not.toContain(tagCount - 2);
    const still = screen.getByRole("textbox", { name: `New name for "${name(0)}"` });
    expect(still).toBe(input);
    expect((still as HTMLInputElement).value).toBe("下書き");
  });

  describe("スクロールした位置から", () => {
    const scrolled = 1000;

    /** scrollWindow は文書を scrolled までスクロールしたことにする。 */
    function scrollWindow() {
      // スクロールの行き先が文書の高さで切り詰められないよう、文書に高さを置く。
      Object.defineProperty(document.documentElement, "scrollHeight", {
        configurable: true,
        value: 100_000,
      });
      Object.defineProperty(window, "scrollY", { configurable: true, value: scrolled });
      window.dispatchEvent(new Event("scroll"));
    }

    afterEach(() => {
      Object.defineProperty(window, "scrollY", { configurable: true, value: 0 });
      Reflect.deleteProperty(document.documentElement, "scrollHeight");
    });

    it("描いていない前の行へ戻るときは、留めた帯の高さを差し引いてスクロールする", async () => {
      const user = userEvent.setup();
      install();
      renderPage();
      await screen.findByTitle(name(0));
      scrollWindow();
      await waitFor(() => expect(Math.min(...drawnIndexes())).toBeGreaterThan(0));

      const first = Math.min(...drawnIndexes());
      screen.getByRole("checkbox", { name: `Select "${name(first)}"` }).focus();
      const scrollTo = vi.mocked(window.scrollTo);
      scrollTo.mockClear();
      await user.tab({ shift: true });

      expect(document.activeElement).toBe(
        within(wrapperOf(first - 1)).getByRole("button", { name: "More actions" }),
      );
      // 行の上端（jsdom では一覧の上端が文書の 0、行の高さは rowHeight）から上部バーの
      // 高さだけ上へ。行が上部バーの下に隠れない（ui-design.md「Keyboard across
      // virtualized rows」）。
      const start = (first - 1) * rowHeight;
      expect(start).toBeGreaterThan(bandHeight);
      expect(scrollTo).toHaveBeenCalledWith(
        expect.objectContaining({ top: start - bandHeight }),
      );
    });

    it("一覧の先頭が帯の下に隠れていれば、「新しいタグ」は先頭まで戻してから作成の行を出す", async () => {
      const user = userEvent.setup();
      // 一覧の先頭は表示域の上の外（帯の下端より上）にある。
      const listTop = -300;
      const rect = vi
        .spyOn(Element.prototype, "getBoundingClientRect")
        .mockReturnValue({ top: listTop } as DOMRect);
      try {
        install();
        renderPage();
        await screen.findByTitle(name(0));
        scrollWindow();
        const scrollTo = vi.mocked(window.scrollTo);
        scrollTo.mockClear();

        await user.click(screen.getByRole("button", { name: "New tag" }));

        expect(scrollTo).toHaveBeenCalledWith({ top: scrolled + listTop - bandHeight });
        await waitFor(() =>
          expect(document.activeElement).toBe(
            screen.getByRole("textbox", { name: "New tag name" }),
          ),
        );
      } finally {
        rect.mockRestore();
      }
    });
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
    const dialog = await screen.findByRole("alertdialog");
    await user.click(within(dialog).getByRole("button", { name: "Delete" }));

    await waitFor(() => expect(screen.queryByTitle(name(index))).toBeNull());
    await waitFor(() =>
      expect(document.activeElement).toBe(
        within(
          screen.getByTitle(name(index + 1)).closest<HTMLElement>("[data-tag-id]")!,
        ).getByRole("button", { name: "Rename" }),
      ),
    );
  });

  it("「Tentative only」中の確定で行が外れると、描いていなかった次の行の「改名」へフォーカスが移る", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle(name(0));
    await toggleFilter(user, "Tentative only");
    const index = await tabPastRange(user);
    expect(document.querySelector(`[data-index="${String(index + 1)}"]`)).toBeNull();

    await user.click(within(wrapperOf(index)).getByRole("button", { name: "Confirm" }));

    await waitFor(() => expect(screen.queryByTitle(name(index))).toBeNull());
    await waitFor(() =>
      expect(document.activeElement).toBe(
        within(
          screen.getByTitle(name(index + 1)).closest<HTMLElement>("[data-tag-id]")!,
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

    expect(await filterChecked(user, "Unused only")).toBe(false);
    await toggleFilter(user, "Unused only");
    expect(await filterChecked(user, "Unused only")).toBe(true);
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
    await toggleFilter(user, "Tentative only");
    expect(await screen.findByText("1 of 5 tags")).toBeDefined();
    expect(rowNames()).toEqual(["Delta"]);

    // 検索と重ねる。
    await user.type(screen.getByRole("searchbox", { name: "Search tags" }), "zz");
    expect(await screen.findByText('No unused tentative tags match "zz"')).toBeDefined();
    expect(screen.getByText("0 of 5 tags")).toBeDefined();
  });

  it("0本のタグが無いときは「No unused tags」を出し、「Show all tags」で外して「Filter」へ戻る", async () => {
    const user = userEvent.setup();
    server.tags = server.tags.map((item) => ({
      ...item,
      videoCount: item.videoCount + 1,
    }));
    install();
    renderPage();
    await screen.findByTitle("Alpha");

    await toggleFilter(user, "Unused only");
    expect(await screen.findByText("No unused tags")).toBeDefined();
    expect(screen.getByText("Every tag is on at least one video.")).toBeDefined();

    await user.click(screen.getByRole("button", { name: "Show all tags" }));
    await waitFor(() => expect(document.activeElement).toBe(filterButton()));
    expect(filterButton().getAttribute("aria-label")).toBe("Filter");
    expect(await screen.findByText("5 tags")).toBeDefined();
  });

  it("「Unused only」と「Tentative only」の両方で一致が無ければ両方を外して「Filter」へ戻る", async () => {
    const user = userEvent.setup();
    server.tags = server.tags.map((item) =>
      item.tentative ? { ...item, videoCount: 2 } : item,
    );
    install();
    renderPage();
    await screen.findByTitle("Alpha");

    await toggleFilter(user, "Unused only");
    await toggleFilter(user, "Tentative only");
    expect(await screen.findByText("No unused tentative tags")).toBeDefined();
    expect(screen.queryByText("Every tag is on at least one video.")).toBeNull();
    expect(filterButton().getAttribute("aria-label")).toBe("Filter (2 applied)");

    await user.click(screen.getByRole("button", { name: "Show all tags" }));
    await waitFor(() => expect(document.activeElement).toBe(filterButton()));
    expect(filterButton().getAttribute("aria-label")).toBe("Filter");
    expect(await filterChecked(user, "Tentative only")).toBe(false);
    expect(await filterChecked(user, "Unused only")).toBe(false);
  });

  it("「Unused only」と検索で一致が無ければ、両方を外して検索の入力へ戻る", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("Alpha");

    await toggleFilter(user, "Unused only");
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
    await toggleFilter(user, "Unused only");
    // Alpha は 5 本で「Unused only」に一致しないが、行は本数の順の位置に残る。
    await waitFor(() => expect(screen.getByText("2 of 5 tags")).toBeDefined());
    const still = screen.getByRole("textbox", { name: 'New name for "Alpha"' });
    expect(still).toBe(input);
    expect((still as HTMLInputElement).value).toBe("下書き");
    expect(document.querySelectorAll("[data-index]")).toHaveLength(3);
  });

  it("読み込み中は並び順と「Filter」が押せない", async () => {
    install();
    holdGetsFrom = 1;
    renderPage();

    const user = userEvent.setup();
    expect((filterButton() as HTMLButtonElement).disabled).toBe(true);
    expect(
      (screen.getByRole("button", { name: "Sort by: Name" }) as HTMLButtonElement)
        .disabled,
    ).toBe(true);
    // 狭い幅のまとめ（「Sort」）の中の並び順も押せない。
    await user.click(screen.getByRole("button", { name: "Sort" }));
    const compact = await screen.findByRole("dialog");
    expect(
      (
        within(compact).getByRole("button", {
          name: "Sort by: Name",
        }) as HTMLButtonElement
      ).disabled,
    ).toBe(true);
    for (const done of heldGetReleases) done();
  });

  it("狭い幅のまとめから並び順を選べる（ライブラリの CompactSortControls と同じ形）", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("Alpha");

    // lg より狭い幅では、並び順はツールバーの「Sort」のポップオーバーにまとまる。
    await user.click(screen.getByRole("button", { name: "Sort" }));
    const compact = await screen.findByRole("dialog");
    // Name のときは向きを出さない。
    expect(
      within(compact).queryByRole("button", { name: /Press for (ascending|descending)/ }),
    ).toBeNull();
    await user.click(within(compact).getByRole("button", { name: "Sort by: Name" }));
    await user.click(await screen.findByRole("menuitemradio", { name: "Video count" }));
    await waitFor(() => expect(rowNames()[0]).toBe("Alpha"));
    await user.click(
      within(compact).getByRole("button", {
        name: "Descending (most videos first). Press for ascending",
      }),
    );
    await waitFor(() => expect(rowNames()[0]).toBe("Beta"));
  });

  it("疑似ロケールで、並び順と「Unused only」の文言がカタログから出る", async () => {
    enablePseudoLocale();
    const user = userEvent.setup();
    install();
    const { container } = renderPage();
    await screen.findByTitle("Alpha");
    await user.click(screen.getByRole("button", { name: /Filter/ }));
    await user.click(await screen.findByRole("checkbox", { name: /Unused only/ }));
    await user.keyboard("{Escape}");
    await screen.findByTitle("Beta");
    expectCatalogTextOnly(container, ["Alpha", "Beta", "Cat", "Delta", "Echo"]);
  });
});

describe("TagsPage まとめての操作", () => {
  function rowOf(name: string): HTMLElement {
    return screen.getByTitle(name).closest("[data-tag-id]")!;
  }

  function selectAll(): HTMLElement {
    return screen.getByRole("checkbox", {
      name: /^Select (all [\d,]+ loaded tags|the 1 loaded tag)$/,
    });
  }

  function bar(): HTMLElement {
    return screen.getByRole("region", { name: "Selected tags" });
  }

  beforeEach(() => {
    server.tags = [
      tag({ id: 1, name: "Alpha", tentative: true, videoCount: 2 }),
      tag({ id: 2, name: "Beta", tentative: true }),
      tag({ id: 3, name: "Cat", videoCount: 4 }),
      tag({ id: 4, name: "Gamma", synonyms: ["ガンマ"], videoCount: 5 }),
    ];
  });

  it("「Tentative only」で読み込んだものをすべて選んで確定すると、全 id を 1 回送り、仮の目印と選択が消え、一覧は取り直さない（受け入れ条件10）", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("Alpha");

    await toggleFilter(user, "Tentative only");
    await waitFor(() => expect(screen.queryByTitle("Cat")).toBeNull());
    expect(server.pageRequests.at(-1)?.get("tentative")).toBe("true");
    const gets = server.getCalls;
    expect(selectAll().getAttribute("aria-label")).toBe("Select all 2 loaded tags");
    await user.click(selectAll());
    expect(within(bar()).getByText("2 tags selected")).toBeDefined();
    expect(screen.getByRole("checkbox", { name: "Clear selection" })).toBeDefined();

    await user.click(within(bar()).getByRole("button", { name: "Confirm" }));

    await waitFor(() =>
      expect(screen.queryByRole("region", { name: "Selected tags" })).toBeNull(),
    );
    expect(server.batchCalls).toEqual([{ action: "confirm", ids: [1, 2] }]);
    expect(await screen.findByText("Confirmed 2 tags")).toBeDefined();
    // 確定は読み込んだ行の中で反映し、ページも全件も取り直さない（R-12）。
    expect(server.getCalls).toBe(gets);
    expect(server.fullGetCalls).toBe(0);
    expect(screen.getByText("0 of 4 tags")).toBeDefined();
    // 確定した行は「Tentative only」から外れ、空になれば「Filter」へ移る。
    await waitFor(() => expect(document.activeElement).toBe(filterButton()));
    await toggleFilter(user, "Tentative only");
    await screen.findByTitle("Alpha");
    expect(screen.queryByText("Tentative")).toBeNull();
  });

  it("仮と確定を混ぜて確定すると、既に確定していた分を数えて伝え、それを選んだまま残す", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("Alpha");

    await user.click(selectAll());
    await user.click(within(bar()).getByRole("button", { name: "Confirm" }));

    expect(
      await screen.findByText("Confirmed 2 tags. 2 were already confirmed."),
    ).toBeDefined();
    expect(within(bar()).getByText("2 tags selected")).toBeDefined();
    // 残った選択は確定したタグだけなので「Confirm」「Reject…」は出さず、フォーカスは
    // 「Merge into one tag…」へ。
    expect(within(bar()).queryByRole("button", { name: "Confirm" })).toBeNull();
    expect(within(bar()).queryByRole("button", { name: "Reject…" })).toBeNull();
    await waitFor(() =>
      expect(document.activeElement).toBe(
        within(bar()).getByRole("button", { name: "Merge into one tag…" }),
      ),
    );
  });

  it("仮と確定を混ぜて削除すると、確認に確定したタグの数と動画の本数が出て、外した数をトーストで伝える（受け入れ条件11）", async () => {
    const user = userEvent.setup();
    server.impactVideoCount = 7;
    install();
    renderPage();
    await screen.findByTitle("Alpha");

    await user.click(selectAll());
    await user.click(within(bar()).getByRole("button", { name: "Delete…" }));

    const dialog = await screen.findByRole("alertdialog", {
      name: "Delete selected tags",
    });
    expect(server.impactCalls).toEqual([{ action: "delete", ids: [1, 2, 3, 4] }]);
    expect(
      await within(dialog).findByText(
        "2 of the 4 selected tags are confirmed. They will be removed from 7 videos. This can't be undone. The 2 tentative tags are left as they are. Reject them instead.",
      ),
    ).toBeDefined();

    await user.click(within(dialog).getByRole("button", { name: "Delete" }));

    expect(
      await screen.findByText("Deleted 2 tags. 2 tentative tags were skipped."),
    ).toBeDefined();
    expect(server.batchCalls).toEqual([{ action: "delete", ids: [1, 2, 3, 4] }]);
    expect(screen.queryByTitle("Cat")).toBeNull();
    expect(screen.queryByTitle("Gamma")).toBeNull();
    // 働かなかった仮のタグは選んだまま。
    expect(within(bar()).getByText("2 tags selected")).toBeDefined();
  });

  it("確認の数が届くまで実行できず、数えられなければ理由と再試行を出す", async () => {
    const user = userEvent.setup();
    server.failImpact = true;
    install();
    renderPage();
    await screen.findByTitle("Alpha");

    await user.click(within(rowOf("Alpha")).getByRole("checkbox"));
    await user.click(within(bar()).getByRole("button", { name: "Reject…" }));
    const dialog = await screen.findByRole("alertdialog", {
      name: "Reject selected tags",
    });
    const reject = within(dialog).getByRole("button", { name: "Reject" });
    expect((reject as HTMLButtonElement).disabled).toBe(true);

    expect(
      (await within(dialog).findByRole("alert")).textContent?.startsWith(
        "Couldn't count the affected videos:",
      ),
    ).toBe(true);
    expect((reject as HTMLButtonElement).disabled).toBe(true);

    server.failImpact = false;
    await user.click(within(dialog).getByRole("button", { name: "Retry" }));
    expect(
      await within(dialog).findByText(
        "The selected tag will be removed from 2 videos, and automatic tagging won't create its name again. You can allow a name again from Rejected names.",
      ),
    ).toBeDefined();
    await user.click(reject);
    expect(await screen.findByText("Rejected 1 tag")).toBeDefined();
    expect(screen.queryByTitle("Alpha")).toBeNull();
  });

  it("対象の一部がもう無ければ一覧を取り直し、失敗すれば選択を残す", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("Alpha");

    await user.click(within(rowOf("Alpha")).getByRole("checkbox"));
    await user.click(within(rowOf("Beta")).getByRole("checkbox"));

    // 失敗は何も変えず、選択は残る。
    server.failNextBatch = true;
    await user.click(within(bar()).getByRole("button", { name: "Confirm" }));
    expect(await screen.findByText(/Something went wrong/)).toBeDefined();
    expect(within(bar()).getByText("2 tags selected")).toBeDefined();
    expect(within(rowOf("Alpha")).getByText("Tentative")).toBeDefined();

    // 別のタブで Beta が消えていた。
    server.tags = server.tags.filter((item) => item.id !== 2);
    const gets = server.getCalls;
    await user.click(within(bar()).getByRole("button", { name: "Confirm" }));
    expect(
      await screen.findByText(
        "Some of the tags no longer existed, so the list was reloaded",
      ),
    ).toBeDefined();
    await waitFor(() => expect(screen.queryByTitle("Beta")).toBeNull());
    expect(server.getCalls).toBeGreaterThan(gets);
    expect(screen.queryByRole("region", { name: "Selected tags" })).toBeNull();
  });

  it("並び順を変えると sort=countDesc で読み直されて選択が空になり、検索を変えても空になる（Edge Case）", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("Alpha");

    await user.click(selectAll());
    expect(within(bar()).getByText("4 tags selected")).toBeDefined();

    await user.click(screen.getByRole("button", { name: "Sort by: Name" }));
    await user.click(await screen.findByRole("menuitemradio", { name: "Video count" }));
    await waitFor(() =>
      expect(server.pageRequests.at(-1)?.get("sort")).toBe("countDesc"),
    );
    expect(server.pageRequests.at(-1)?.get("limit")).toBe("100");
    expect(screen.queryByRole("region", { name: "Selected tags" })).toBeNull();
    // 読み直した行が同じ id を持っていても選び直さない。
    await waitFor(() =>
      expect(screen.getAllByRole("link").map((link) => link.textContent)).toEqual([
        "Gamma",
        "Cat",
        "Alpha",
        "Beta",
      ]),
    );
    expect(selectAll().getAttribute("aria-checked")).toBe("false");

    await user.click(within(rowOf("Alpha")).getByRole("checkbox"));
    expect(within(bar()).getByText("1 tag selected")).toBeDefined();
    await user.type(screen.getByRole("searchbox", { name: "Search tags" }), "a");
    await waitFor(() =>
      expect(screen.queryByRole("region", { name: "Selected tags" })).toBeNull(),
    );
  });

  it("選んでいる行の改名を始めると選択から外れ、改名中の行のチェックは押せない", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("Alpha");

    await user.click(within(rowOf("Alpha")).getByRole("checkbox"));
    await user.click(within(rowOf("Alpha")).getByRole("button", { name: "Rename" }));
    await screen.findByRole("textbox", { name: 'New name for "Alpha"' });
    expect(screen.queryByRole("region", { name: "Selected tags" })).toBeNull();
    const check = screen.getByRole("checkbox", { name: 'Select "Alpha"' });
    expect((check as HTMLButtonElement).disabled).toBe(true);
  });

  it("選んでいる間はページの下端に選択バーを出し、働かない操作は出さない。×で解くとバーが消え、先頭のチェックへフォーカスが移る", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("Alpha");
    expect(screen.getByRole("heading", { name: "Tags" })).toBeDefined();

    // 確定したタグだけを選ぶと「Confirm」「Reject…」は出さない（薄くもしない）。
    await user.click(within(rowOf("Cat")).getByRole("checkbox"));
    expect(within(bar()).getByText("1 tag selected")).toBeDefined();
    expect(
      within(bar())
        .getAllByRole("button")
        .map((button) => button.getAttribute("aria-label") ?? button.textContent),
    ).toEqual(["Clear selection", "Merge into one tag…", "Delete…"]);

    // 仮のタグも選ぶと「Confirm」「Reject…」が出る。
    await user.click(within(rowOf("Alpha")).getByRole("checkbox"));
    expect(
      within(bar())
        .getAllByRole("button")
        .map((button) => button.getAttribute("aria-label") ?? button.textContent),
    ).toEqual([
      "Clear selection",
      "Confirm",
      "Merge into one tag…",
      "Reject…",
      "Delete…",
    ]);
    // 選択バーはデザインシステムの SelectionBar で、ページの下端に貼り付く。
    expect(bar().querySelector('[data-slot="selection-bar"]')).not.toBeNull();

    await user.click(within(bar()).getByRole("button", { name: "Clear selection" }));
    expect(screen.queryByRole("region", { name: "Selected tags" })).toBeNull();
    expect(screen.getByRole("heading", { name: "Tags" })).toBeDefined();
    await waitFor(() => expect(document.activeElement).toBe(selectAll()));
  });

  it("選んだ行は表の選択の面（primary-soft）で塗る", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("Alpha");

    await user.click(within(rowOf("Cat")).getByRole("checkbox"));
    // 表の選択した行（data-state="selected"）は primary-soft の面になる。
    expect(rowOf("Cat").getAttribute("data-state")).toBe("selected");
    expect(rowOf("Cat").className).toContain("data-[state=selected]:bg-primary-soft");
    expect(rowOf("Gamma").getAttribute("data-state")).toBeNull();
  });

  it("タッチ・狭い幅の「Actions」は文字を持つ項目を並べ、「Confirm」は行の「確定する」と同じ結果になる", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("Alpha");

    await user.click(within(rowOf("Alpha")).getByRole("button", { name: "Actions" }));
    const items = (await screen.findAllByRole("menuitem")).map(
      (item) => item.textContent,
    );
    expect(items).toEqual([
      "Confirm",
      "Rename",
      "Synonyms",
      "Merge into another tag…",
      "Reject…",
    ]);
    await user.click(screen.getByRole("menuitem", { name: "Confirm" }));

    expect(await screen.findByText('Confirmed "Alpha"')).toBeDefined();
    expect(server.confirmCalls).toBe(1);
    await waitFor(() =>
      expect(within(rowOf("Alpha")).queryByText("Tentative")).toBeNull(),
    );

    await user.click(within(rowOf("Cat")).getByRole("button", { name: "Actions" }));
    expect(
      (await screen.findAllByRole("menuitem")).map((item) => item.textContent),
    ).toEqual(["Rename", "Synonyms", "Merge into another tag…", "Delete…"]);
  });

  it("疑似ロケールで、選択・選択バー・まとめての確認の文言がカタログから出る", async () => {
    const user = userEvent.setup();
    enablePseudoLocale();
    install();
    const { container } = renderPage();
    await screen.findByTitle("Alpha");

    // Alpha（仮のタグ）を選ぶ。選択の行のボタンは ×・Confirm・Merge・Reject の順。
    await user.click(
      container.querySelector<HTMLElement>('[data-tag-id] [role="checkbox"]')!,
    );
    expectCatalogTextOnly(document.body, ["Alpha", "Beta", "Cat", "Gamma", "ガンマ"]);
    await user.click(
      within(
        screen
          .getAllByRole("region")
          // 通知（Sonner）の section も region なので外す。
          .find((region) => region.tagName !== "SECTION")!,
      ).getAllByRole("button")[3]!,
    );
    const dialog = await screen.findByRole("alertdialog");
    await waitFor(() => expect(server.impactCalls).toHaveLength(1));
    await within(dialog).findByText(/videos|video/i);
    expectCatalogTextOnly(document.body, ["Alpha", "Beta", "Cat", "Gamma", "ガンマ"]);
  });
});

describe("TagsPage まとめての統合", () => {
  function rowOf(name: string): HTMLElement {
    return screen.getByTitle(name).closest("[data-tag-id]")!;
  }

  function bar(): HTMLElement {
    return screen.getByRole("region", { name: "Selected tags" });
  }

  beforeEach(() => {
    server.tags = [
      tag({ id: 1, name: "Action", videoCount: 10 }),
      tag({ id: 2, name: "Alpha", tentative: true, videoCount: 2 }),
      tag({ id: 3, name: "Beta", tentative: true }),
      tag({ id: 4, name: "Cat", videoCount: 4 }),
      tag({ id: 5, name: "Gamma", videoCount: 5 }),
    ];
  });

  async function openMerge(
    user: ReturnType<typeof userEvent.setup>,
    names: readonly string[],
  ): Promise<HTMLElement> {
    for (const name of names) {
      await user.click(within(rowOf(name)).getByRole("checkbox"));
    }
    await user.click(within(bar()).getByRole("button", { name: "Merge into one tag…" }));
    return screen.findByRole("dialog");
  }

  async function chooseTarget(
    user: ReturnType<typeof userEvent.setup>,
    dialog: HTMLElement,
    name: string,
  ) {
    const combo = within(dialog).getByRole("combobox", { name: "Tag to merge into" });
    await user.clear(combo);
    await user.type(combo, name);
    await user.click(
      await within(dialog).findByRole("option", { name: new RegExp(`^${name}`) }),
    );
  }

  it("複数から開くと見出しが「Merge 4 tags」で統合元のチップが並び、統合先の入力と候補の一覧は窓の幅いっぱい（要件13）", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("Alpha");

    const dialog = await openMerge(user, ["Alpha", "Beta", "Cat", "Gamma"]);
    expect(screen.getByRole("dialog", { name: "Merge 4 tags" })).toBe(dialog);
    const chips = within(dialog).getByRole("list", { name: "Tags to merge" });
    expect(
      within(chips)
        .getAllByRole("listitem")
        .map((item) => item.textContent),
    ).toEqual(["AlphaTentative", "BetaTentative", "Cat", "Gamma"]);

    const combo = within(dialog).getByRole("combobox", { name: "Tag to merge into" });
    expect(combo.parentElement!.className).toContain("w-full");
    expect(combo.parentElement!.className).not.toContain("w-40");
    await user.type(combo, "A");
    // 候補の一覧は入力の下の本文の中にあり、入力に重ねない。
    const listbox = await within(dialog).findByRole("listbox");
    expect(listbox.className).not.toContain("absolute");
    // 統合先の候補は全タグ（選んだ中からも、選んでいないタグからも）。
    expect(within(listbox).getByRole("option", { name: /^Action/ })).toBeDefined();
    expect(within(listbox).getByRole("option", { name: /^Alpha/ })).toBeDefined();
  });

  it("統合先を選ぶと数を待ち、届いたら tagCount・videoCount の確認を出して統合する", async () => {
    const user = userEvent.setup();
    server.impactVideoCount = 9;
    const fetchMock = install();
    renderPage();
    await screen.findByTitle("Alpha");

    const dialog = await openMerge(user, ["Alpha", "Beta", "Cat", "Gamma"]);
    await chooseTarget(user, dialog, "Action");

    expect(
      await within(dialog).findByText(
        'The 9 videos tagged with these 4 tags get the tag "Action". Their names and synonyms become synonyms of "Action", and the 4 tags leave the tag list. This can\'t be undone.',
      ),
    ).toBeDefined();
    expect(server.impactCalls).toEqual([{ action: "merge", ids: [2, 3, 4, 5] }]);
    const mergeButton = within(dialog).getByRole("button", { name: "Merge" });
    await waitFor(() => expect(document.activeElement).toBe(mergeButton));

    await user.click(mergeButton);

    expect(await screen.findByText('Merged 4 tags into "Action"')).toBeDefined();
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/tags/1/merge",
      expect.objectContaining({ body: JSON.stringify({ sourceIds: [2, 3, 4, 5] }) }),
    );
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    for (const name of ["Alpha", "Beta", "Cat", "Gamma"]) {
      expect(screen.queryByTitle(name)).toBeNull();
    }
    // 本数は応答の tag の videoCount（このモックでは合算の 21）。
    expect(within(rowOf("Action")).getByText("21 videos")).toBeDefined();
    expect(screen.queryByRole("region", { name: "Selected tags" })).toBeNull();
    await waitFor(() =>
      expect(document.activeElement).toBe(
        within(rowOf("Action")).getByRole("link", { name: /Action/ }),
      ),
    );
  });

  it("選んだ中のタグを統合先にすると統合元から外れ、統合先だけなら実行できない（Edge Case）", async () => {
    const user = userEvent.setup();
    const fetchMock = install();
    renderPage();
    await screen.findByTitle("Alpha");

    const dialog = await openMerge(user, ["Action", "Alpha", "Cat", "Gamma"]);
    await chooseTarget(user, dialog, "Action");

    expect(
      within(dialog).getByText('"Action" is kept and the other 3 tags merge into it.'),
    ).toBeDefined();
    const chips = within(dialog).getByRole("list", { name: "Tags to merge" });
    expect(within(chips).getByText("kept")).toBeDefined();
    expect(
      await within(dialog).findByText(
        /^The 11 videos tagged with these 3 tags get the tag "Action"\./,
      ),
    ).toBeDefined();
    expect(server.impactCalls.at(-1)).toEqual({ action: "merge", ids: [2, 4, 5] });

    await user.click(within(dialog).getByRole("button", { name: "Merge" }));
    expect(await screen.findByText('Merged 3 tags into "Action"')).toBeDefined();
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/tags/1/merge",
      expect.objectContaining({ body: JSON.stringify({ sourceIds: [2, 4, 5] }) }),
    );
  });

  it("選んだのが 1 件でそれを統合先にすると「Merge」は押せない", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("Alpha");

    const dialog = await openMerge(user, ["Cat"]);
    within(dialog).getByText('Merge "Cat"');
    await chooseTarget(user, dialog, "Cat");

    expect(
      within(dialog).getByText(
        'Choose another tag to merge into: "Cat" is the only tag selected.',
      ),
    ).toBeDefined();
    expect(
      (within(dialog).getByRole("button", { name: "Merge" }) as HTMLButtonElement)
        .disabled,
    ).toBe(true);
    expect(server.impactCalls).toEqual([]);

    await user.click(within(dialog).getByRole("button", { name: "Cancel" }));
    await waitFor(() =>
      expect(document.activeElement).toBe(
        within(bar()).getByRole("button", { name: "Merge into one tag…" }),
      ),
    );
  });

  it("数えられなければ理由と再試行を出し、統合元がすべてもう無ければ統合のトーストを出さずに取り直す", async () => {
    const user = userEvent.setup();
    server.failImpact = true;
    install();
    renderPage();
    await screen.findByTitle("Alpha");

    const dialog = await openMerge(user, ["Cat", "Gamma"]);
    await chooseTarget(user, dialog, "Action");
    expect(
      (await within(dialog).findByRole("alert")).textContent?.startsWith(
        "Couldn't count the affected videos:",
      ),
    ).toBe(true);
    const mergeButton = within(dialog).getByRole("button", { name: "Merge" });
    expect((mergeButton as HTMLButtonElement).disabled).toBe(true);

    server.failImpact = false;
    await user.click(within(dialog).getByRole("button", { name: "Retry" }));
    await within(dialog).findByText(/these 2 tags/);

    // 別のタブで Cat と Gamma が消えていた。
    server.tags = server.tags.filter((item) => item.id !== 4 && item.id !== 5);
    const gets = server.getCalls;
    await user.click(mergeButton);
    expect(
      await screen.findByText(
        "Some of the tags no longer existed, so the list was reloaded",
      ),
    ).toBeDefined();
    expect(screen.queryByText(/^Merged/)).toBeNull();
    await waitFor(() => expect(screen.queryByTitle("Cat")).toBeNull());
    expect(server.getCalls).toBeGreaterThan(gets);
  });

  it("統合元の一部がもう無ければ、実際に統合した数を伝えて一覧を取り直す", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("Alpha");

    const dialog = await openMerge(user, ["Alpha", "Cat", "Gamma"]);
    await chooseTarget(user, dialog, "Action");
    await within(dialog).findByText(/these 3 tags/);

    server.tags = server.tags.filter((item) => item.id !== 5);
    await user.click(within(dialog).getByRole("button", { name: "Merge" }));
    expect(await screen.findByText('Merged 2 tags into "Action"')).toBeDefined();
    expect(
      await screen.findByText(
        "Some of the tags no longer existed, so the list was reloaded",
      ),
    ).toBeDefined();
    await waitFor(() => expect(screen.queryByTitle("Gamma")).toBeNull());
  });
});

describe("TagsPage まとめての操作の上限", () => {
  let originalOffsetHeight: PropertyDescriptor | undefined;

  beforeEach(() => {
    originalOffsetHeight = Object.getOwnPropertyDescriptor(
      HTMLElement.prototype,
      "offsetHeight",
    );
    Object.defineProperty(HTMLElement.prototype, "offsetHeight", {
      configurable: true,
      get(this: HTMLElement) {
        return this.hasAttribute("data-index") ? 40 : 0;
      },
    });
  });

  afterEach(() => {
    if (originalOffsetHeight !== undefined) {
      Object.defineProperty(HTMLElement.prototype, "offsetHeight", originalOffsetHeight);
    }
  });

  it("読み込んだ行が上限を超えると先頭のチェックだけが押せず、1 行ずつ選んだ数行のまとめての操作は押せる", async () => {
    const user = userEvent.setup();
    server.tags = Array.from({ length: 20002 }, (_, index) =>
      tag({ id: index + 1, name: `T${String(index).padStart(5, "0")}`, tentative: true }),
    );
    // 続きを何度も読んだあとと同じく、上限を超える行を読み込んだ状態を 1 回の応答で作る。
    server.pageLimitOverride = 20001;
    install();
    renderPage();
    await screen.findByTitle("T00000");

    const reason =
      "Too many tags are loaded to select them all at once (limit 20,000). Narrow the list with search or a filter.";
    const header = screen.getByRole("checkbox", {
      name: "Select all 20,001 loaded tags",
    });
    expect((header as HTMLButtonElement).disabled).toBe(true);
    expect(header.parentElement?.getAttribute("title")).toBe(reason);
    const describedBy = header.getAttribute("aria-describedby");
    expect(describedBy).not.toBeNull();
    expect(document.getElementById(describedBy!)?.textContent).toBe(reason);

    await user.click(screen.getByRole("checkbox", { name: 'Select "T00000"' }));
    await user.click(screen.getByRole("checkbox", { name: 'Select "T00001"' }));
    const region = screen.getByRole("region", { name: "Selected tags" });
    for (const name of ["Confirm", "Merge into one tag…", "Reject…"]) {
      const button = within(region).getByRole("button", { name });
      expect((button as HTMLButtonElement).disabled).toBe(false);
    }
    await user.click(within(region).getByRole("button", { name: "Confirm" }));
    expect(await screen.findByText("Confirmed 2 tags")).toBeDefined();
    expect(server.batchCalls).toEqual([{ action: "confirm", ids: [1, 2] }]);
  }, 60000);
});

describe("TagsPage サーバーのページで読む（specs/036-tag-admin-scale/research.md R-1・R-11・R-12）", () => {
  /** jsdom には表示域の高さと要素の高さが無いので、試験用の高さを置く。 */
  const rowHeight = 40;
  const viewportHeight = 200;
  let originalInnerHeight = 0;
  let originalOffsetHeight: PropertyDescriptor | undefined;

  /** name は並べたときに番号の順になる名前（名前の自然順）。 */
  function name(index: number): string {
    return `Tag ${String(index)}`;
  }

  function loadedNames(): string[] {
    return [...document.querySelectorAll<HTMLElement>("[data-index]")]
      .sort((a, b) => Number(a.dataset.index) - Number(b.dataset.index))
      .map(
        (row) => row.querySelector("[data-tag-id] [title]")?.getAttribute("title") ?? "",
      );
  }

  function count(): string {
    return screen
      .getAllByRole("status")
      .find((node) => node.closest('[data-slot="page-header-count"]') !== null)!
      .textContent!;
  }

  function rowOf(tagName: string): HTMLElement {
    return screen.getByTitle(tagName).closest("[data-tag-id]")!;
  }

  /** scrollTo は文書を top までスクロールしたことにし、仮想化に知らせる。 */
  function scrollTo(top: number) {
    Object.defineProperty(document.documentElement, "scrollHeight", {
      configurable: true,
      value: 1_000_000,
    });
    Object.defineProperty(window, "scrollY", { configurable: true, value: top });
    window.dispatchEvent(new Event("scroll"));
  }

  /** scrollToEnd は読み込んだ行の末尾近くまでスクロールし、続きのきっかけを作る。 */
  async function scrollToEnd(loaded: number) {
    scrollTo(loaded * 60);
    await waitFor(() =>
      expect(
        document.querySelector(`[data-index="${String(loaded - 1)}"]`),
      ).not.toBeNull(),
    );
  }

  function cursorRequests(): URLSearchParams[] {
    return server.pageRequests.filter((params) => params.has("cursor"));
  }

  beforeEach(() => {
    server.tags = Array.from({ length: 150 }, (_, index) =>
      tag({ id: index + 1, name: name(index), tentative: index % 2 === 0 }),
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
    Object.defineProperty(window, "scrollY", { configurable: true, value: 0 });
    Reflect.deleteProperty(document.documentElement, "scrollHeight");
  });

  it("開くと GET /api/tags を limit=100&sort=name で 1 回だけ送り、全件は送らない", async () => {
    install();
    renderPage();
    await screen.findByTitle(name(0));

    expect(server.pageRequests.map((params) => params.toString())).toEqual([
      "sort=name&limit=100",
    ]);
    expect(server.fullGetCalls).toBe(0);
    // 件数は全部の数で、続きがあっても読み込んだ数（ページの区切り）は出さない。
    expect(count()).toBe("150 tags");
    expect(document.body.textContent).not.toContain("loaded");
    await new Promise((resolve) => setTimeout(resolve, 20));
    expect(server.getCalls).toBe(1);
  });

  it("検索は q で先頭から読み直し、応答の行だけが並ぶ（受け入れ条件9）", async () => {
    const user = userEvent.setup();
    server.tags.push(tag({ id: 999, name: "zz action" }));
    install();
    renderPage();
    await screen.findByTitle(name(0));
    expect(screen.queryByTitle("zz action")).toBeNull();

    await user.type(
      screen.getByRole("searchbox", { name: "Search tags" }),
      "ＡＣＴＩＯＮ",
    );

    await screen.findByTitle("zz action");
    await waitFor(() => expect(loadedNames()).toEqual(["zz action"]));
    expect(server.pageRequests.at(-1)?.get("q")).toBe("ＡＣＴＩＯＮ");
    expect(server.pageRequests.at(-1)?.has("cursor")).toBe(false);
    expect(count()).toBe("1 of 151 tags");
    // 入力は q の上限の 100 文字で止まる。
    const search = screen.getByRole("searchbox", { name: "Search tags" });
    await user.clear(search);
    await user.click(search);
    await user.paste("a".repeat(101));
    expect((search as HTMLInputElement).value).toBe("a".repeat(100));
  });

  it("「Unused only」で unused=true を送り、件数の行が total of totalAll になる（受け入れ条件8）", async () => {
    const user = userEvent.setup();
    server.tags = server.tags.map((item, index) => ({
      ...item,
      videoCount: index < 140 ? 1 : 0,
    }));
    install();
    renderPage();
    await screen.findByTitle(name(0));

    await toggleFilter(user, "Unused only");
    await screen.findByTitle(name(140));
    expect(server.pageRequests.at(-1)?.get("unused")).toBe("true");
    // 読み込んでいなかったタグも数えた数で、末尾まで読んだので「loaded」は無い。
    expect(count()).toBe("10 of 150 tags");
    expect(loadedNames()).toHaveLength(10);
  });

  it("末尾に近づくと cursor 付きの要求を 1 回送り、行を末尾に足して重複する id を捨てる", async () => {
    install();
    renderPage();
    await screen.findByTitle(name(0));
    // 読み込んだあとで、読み込んだ行の 1 つが別のタブで改名され、境より後ろに並ぶ
    // ようになった（続きのページにも同じ id が出る）。
    server.tags = server.tags.map((item) =>
      item.id === 6 ? { ...item, name: "Tag 120a" } : item,
    );

    await scrollToEnd(100);
    await waitFor(() => expect(count()).toBe("150 tags"));
    expect(cursorRequests()).toHaveLength(1);
    expect(cursorRequests()[0]?.get("limit")).toBe("100");
    expect(cursorRequests()[0]?.get("sort")).toBe("name");

    scrollTo(150 * 60);
    await waitFor(() =>
      expect(document.querySelector('[data-index="148"]')).not.toBeNull(),
    );
    // 150 行のまま（id 6 は二重に出ない）。続きはもう無いので要求しない。
    expect(document.querySelector('[data-index="149"]')).not.toBeNull();
    expect(document.querySelector('[data-index="150"]')).toBeNull();
    expect(screen.queryByTitle("Tag 120a")).toBeNull();
    expect(cursorRequests()).toHaveLength(1);
  });

  it("続きの応答の totalAll が違えば「一覧が変わった」を出して続きを止め、「Reload」で先頭から読み直す（Edge Case）", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle(name(0));
    await user.click(within(rowOf(name(0))).getByRole("checkbox"));

    // 別のタブでタグが増えた。
    server.tags.push(tag({ id: 500, name: "Tag 999" }));
    await scrollToEnd(100);

    expect(
      await screen.findByText(
        "Tags were added or removed elsewhere, so the rest of this list may be out of date.",
      ),
    ).toBeDefined();
    // 行と選択は残り、続きは足さず、それ以上要求しない。選んでいる間は見出しの件数の
    // 代わりにタブの件数を見る。
    expect(tagsTab().textContent).toBe("Tags150");
    expect(
      within(screen.getByRole("region", { name: "Selected tags" })).getByText(
        "1 tag selected",
      ),
    ).toBeDefined();
    await new Promise((resolve) => setTimeout(resolve, 20));
    expect(cursorRequests()).toHaveLength(1);

    const gets = server.getCalls;
    scrollTo(0);
    await user.click(screen.getByRole("button", { name: "Reload" }));
    await waitFor(() => expect(count()).toBe("151 tags"));
    expect(server.getCalls).toBe(gets + 1);
    expect(server.pageRequests.at(-1)?.has("cursor")).toBe(false);
    expect(screen.queryByRole("region", { name: "Selected tags" })).toBeNull();
    expect(screen.queryByText(/Tags were added or removed elsewhere/)).toBeNull();
  });

  it("続きの読み込みに失敗すると読み込んだ行が残り、「Retry」で同じ cursor を送る（Edge Case）", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle(name(0));

    server.failNextGet = true;
    await scrollToEnd(100);
    const alert = await screen.findByRole("alert");
    expect(alert.textContent).toMatch(/^Couldn't load more: /);
    expect(screen.getByTitle(name(99))).toBeDefined();
    expect(count()).toBe("150 tags");

    await user.click(within(alert).getByRole("button", { name: "Retry" }));
    await waitFor(() => expect(count()).toBe("150 tags"));
    const sent = cursorRequests();
    expect(sent).toHaveLength(2);
    expect(sent[1]?.get("cursor")).toBe(sent[0]?.get("cursor"));
    expect(screen.queryByRole("alert")).toBeNull();
  });

  it("続きを待つ間に検索を変えると、古い条件の続きは一覧に混ざらない（Edge Case）", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle(name(0));

    // 続き（2 回目の要求）から止める。
    holdGetsFrom = 2;
    await scrollToEnd(100);
    await waitFor(() => expect(heldGetReleases).toHaveLength(1));
    expect(screen.getByRole("status", { name: "Loading more tags…" })).toBeDefined();

    await user.type(screen.getByRole("searchbox", { name: "Search tags" }), "Tag 14");
    await waitFor(() => expect(heldGetReleases.length).toBeGreaterThan(1));
    scrollTo(0);
    // 新しい条件の応答を先に、古い続きの応答をあとに解く。
    for (const release of heldGetReleases.slice(1)) release();
    heldGetReleases[0]!();
    await waitFor(() =>
      expect(loadedNames()).toEqual([
        "Tag 14",
        "Tag 140",
        "Tag 141",
        "Tag 142",
        "Tag 143",
        "Tag 144",
        "Tag 145",
        "Tag 146",
        "Tag 147",
        "Tag 148",
        "Tag 149",
      ]),
    );
    await new Promise((resolve) => setTimeout(resolve, 20));
    expect(loadedNames()).toHaveLength(11);
    expect(count()).toBe("11 of 150 tags");
    expect(screen.queryByText(/Couldn't load more/)).toBeNull();
  });

  it("先頭のチェックは読み込んだ行だけを選び、続きがあってもそれ以上は選ばない（要件10）", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle(name(0));

    const header = screen.getByRole("checkbox", { name: "Select all 100 loaded tags" });
    await user.click(header);
    const bar = screen.getByRole("region", { name: "Selected tags" });
    expect(within(bar).getByText("100 tags selected")).toBeDefined();
    expect(header.getAttribute("aria-checked")).toBe("true");

    // 続きが届くと読み込んだ行が増え、全部の状態は中間に戻る。
    await scrollToEnd(100);
    await waitFor(() =>
      expect(
        screen.getByRole("checkbox", { name: "Select all 150 loaded tags" }),
      ).toBeDefined(),
    );
    expect(within(bar).getByText("100 tags selected")).toBeDefined();
    expect(
      screen
        .getByRole("checkbox", { name: "Select all 150 loaded tags" })
        .getAttribute("aria-checked"),
    ).toBe("mixed");
  });

  it("名前の順で作成したタグは名前の自然順の鍵の位置に入る（受け入れ条件6）", async () => {
    const user = userEvent.setup();
    server.tags = [
      tag({ id: 1, name: "B1" }),
      tag({ id: 2, name: "B2" }),
      tag({ id: 3, name: "B10" }),
    ];
    install();
    renderPage();
    await screen.findByTitle("B10");
    expect(loadedNames()).toEqual(["B1", "B2", "B10"]);

    await user.click(screen.getByRole("button", { name: "New tag" }));
    await user.type(screen.getByRole("textbox", { name: "New tag name" }), "ｂ５");
    await user.keyboard("{Enter}");
    await screen.findByTitle("ｂ５");
    expect(loadedNames()).toEqual(["B1", "B2", "ｂ５", "B10"]);
    expect(count()).toBe("4 tags");
    expect(server.getCalls).toBe(1);
  });

  it("検索中に改名して一致しなくなった行は取り除かれ、total が減る", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle(name(0));
    await user.type(screen.getByRole("searchbox", { name: "Search tags" }), "Tag 14");
    await waitFor(() => expect(count()).toBe("11 of 150 tags"));

    await user.click(within(rowOf("Tag 141")).getByRole("button", { name: "Rename" }));
    const input = await screen.findByRole("textbox", { name: 'New name for "Tag 141"' });
    await user.clear(input);
    await user.type(input, "Renamed");
    await user.keyboard("{Enter}");

    await waitFor(() => expect(screen.queryByTitle("Renamed")).toBeNull());
    expect(screen.queryByTitle("Tag 141")).toBeNull();
    expect(count()).toBe("10 of 150 tags");
  });

  it("先頭のページの失敗で、一覧を持っていなければ失敗の表示と「Retry」になる（Edge Case）", async () => {
    const user = userEvent.setup();
    server.failNextGet = true;
    install();
    renderPage();
    expect(await screen.findByText("Couldn't load the tags")).toBeDefined();

    await user.click(screen.getByRole("button", { name: "Retry" }));
    await screen.findByTitle(name(0));
    expect(screen.queryByText("Couldn't load the tags")).toBeNull();
  });

  it("先頭のページの失敗で、一覧を持っていればその一覧を残して帯の中に知らせ、その間は続きを読まない（Edge Case）", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle(name(0));

    server.failNextGet = true;
    await toggleFilter(user, "Unused only");
    const alert = await screen.findByRole("alert");
    expect(alert.textContent).toMatch(
      /^Couldn't load tags: .*\. The list below may not match the current search, filters and sort\./,
    );
    // 前の行と件数を残す。
    expect(screen.getByTitle(name(0))).toBeDefined();
    expect(count()).toBe("150 tags");

    // 箱がある間は、末尾に近づいても前の条件のカーソルで続きを読まない。
    await scrollToEnd(100);
    await new Promise((resolve) => setTimeout(resolve, 20));
    expect(cursorRequests()).toHaveLength(0);

    await user.click(within(alert).getByRole("button", { name: "Retry" }));
    await waitFor(() => expect(count()).toBe("150 of 150 tags"));
    expect(server.pageRequests.at(-1)?.get("unused")).toBe("true");
    expect(screen.queryByRole("alert")).toBeNull();
  });

  it("「Tentative only」で読み込んだ行を全部確定すると、空の状態を出さずにその場で続きを読み、残りの仮のタグが仮のまま出る（受け入れ条件10）", async () => {
    const user = userEvent.setup();
    server.tags = Array.from({ length: 250 }, (_, index) =>
      tag({ id: index + 1, name: name(index), tentative: index % 2 === 0 }),
    );
    install();
    renderPage();
    await screen.findByTitle(name(0));
    await toggleFilter(user, "Tentative only");
    await waitFor(() => expect(count()).toBe("125 of 250 tags"));

    await user.click(
      screen.getByRole("checkbox", { name: "Select all 100 loaded tags" }),
    );
    await user.click(
      within(screen.getByRole("region", { name: "Selected tags" })).getByRole("button", {
        name: "Confirm",
      }),
    );

    expect(await screen.findByText("Confirmed 100 tags")).toBeDefined();
    expect(server.batchCalls[0]?.ids).toHaveLength(100);
    await waitFor(() => expect(count()).toBe("25 of 250 tags"));
    expect(cursorRequests()).toHaveLength(1);
    expect(screen.queryByText("No tentative tags")).toBeNull();
    expect(loadedNames()[0]).toBe(name(200));
    expect(within(rowOf(name(200))).getByText("Tentative")).toBeDefined();
  });

  it("先頭のページを待つ間に削除すると、削除の前に送った応答は捨てて読み直し、消したタグが戻らない（R-12）", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle(name(1));

    // 「Unused only」の先頭のページ（2 回目の要求）から止める。全部のタグが 0 本。
    holdGetsFrom = 2;
    await toggleFilter(user, "Unused only");
    await waitFor(() => expect(heldGetReleases).toHaveLength(1));

    await user.click(
      within(rowOf(name(1))).getByRole("button", { name: "More actions" }),
    );
    await user.click(await screen.findByRole("menuitem", { name: "Delete…" }));
    const dialog = await screen.findByRole("alertdialog", {
      name: `Delete "${name(1)}"`,
    });
    await user.click(within(dialog).getByRole("button", { name: "Delete" }));
    await waitFor(() => expect(screen.queryByTitle(name(1))).toBeNull());

    // 削除の前に受けた先頭のページ（消したタグを含む）が届く。
    heldGetReleases[0]!();
    await waitFor(() => expect(heldGetReleases).toHaveLength(2));
    expect(server.pageRequests.at(-1)?.get("unused")).toBe("true");
    expect(server.pageRequests.at(-1)?.has("cursor")).toBe(false);
    expect(screen.queryByTitle(name(1))).toBeNull();

    heldGetReleases[1]!();
    await waitFor(() => expect(count()).toBe("149 of 149 tags"));
    expect(screen.queryByTitle(name(1))).toBeNull();
  });

  it("続きを待つ間に確定すると、確定の前に送った続きは捨てて同じ cursor で読み直し、件数が戻らない（R-12）", async () => {
    const user = userEvent.setup();
    server.tags = Array.from({ length: 250 }, (_, index) =>
      tag({ id: index + 1, name: name(index), tentative: index % 2 === 0 }),
    );
    install();
    renderPage();
    await screen.findByTitle(name(0));
    await toggleFilter(user, "Tentative only");
    await waitFor(() => expect(count()).toBe("125 of 250 tags"));

    holdGetsFrom = server.getCalls + 1;
    await scrollToEnd(100);
    await waitFor(() => expect(heldGetReleases).toHaveLength(1));
    scrollTo(0);
    await screen.findByTitle(name(0));

    await user.click(within(rowOf(name(0))).getByRole("button", { name: "Confirm" }));
    await waitFor(() => expect(screen.queryByTitle(name(0))).toBeNull());
    expect(count()).toBe("124 of 250 tags");

    // 確定の前に受けた続き（total 125）が届く。
    heldGetReleases[0]!();
    await waitFor(() => expect(heldGetReleases).toHaveLength(2));
    const sent = cursorRequests();
    expect(sent).toHaveLength(2);
    expect(sent[1]?.get("cursor")).toBe(sent[0]?.get("cursor"));
    expect(count()).toBe("124 of 250 tags");

    heldGetReleases[1]!();
    await waitFor(() => expect(count()).toBe("124 of 250 tags"));
    expect(screen.queryByText(/Tags were added or removed elsewhere/)).toBeNull();
  });

  it("並び順を変えて読み込んだ範囲の外になった改名中の行を改名しても、件数は増えない", async () => {
    const user = userEvent.setup();
    // 本数の多い順では Tag 0（0 本）が最後に並び、先頭のページに入らない。
    server.tags = server.tags.map((item, index) => ({
      ...item,
      tentative: false,
      videoCount: index,
    }));
    install();
    renderPage();
    await screen.findByTitle(name(0));
    // 件数の行が total を出すよう、全部に合う検索で絞る。
    await user.type(screen.getByRole("searchbox", { name: "Search tags" }), "Tag");
    await waitFor(() => expect(count()).toBe("150 of 150 tags"));

    await user.click(within(rowOf(name(0))).getByRole("button", { name: "Rename" }));
    await screen.findByRole("textbox", { name: `New name for "${name(0)}"` });
    // 並び順を変えた先頭のページの次（改名中の行が末尾にあるので続きを読む）は届かない
    // ままにし、改名中の行を読み込んでいない間に改名する。
    holdGetsFrom = server.getCalls + 2;
    await user.click(screen.getByRole("button", { name: "Sort by: Name" }));
    await user.click(await screen.findByRole("menuitemradio", { name: "Video count" }));
    await waitFor(() =>
      expect(server.pageRequests.at(-1)?.get("sort")).toBe("countDesc"),
    );
    await waitFor(() => expect(loadedNames()[0]).toBe(name(149)));
    expect(count()).toBe("150 of 150 tags");

    const input = screen.getByRole("textbox", { name: `New name for "${name(0)}"` });
    await user.clear(input);
    await user.type(input, "Tag renamed{Enter}");
    await waitFor(() =>
      expect(server.tags.find((item) => item.id === 1)?.name).toBe("Tag renamed"),
    );
    await waitFor(() =>
      expect(
        screen.queryByRole("textbox", { name: `New name for "${name(0)}"` }),
      ).toBeNull(),
    );
    expect(count()).toBe("150 of 150 tags");
  });

  it("シノニム登録に伴って読み込んでいないタグを統合すると、条件に合っていたそのタグを total からも引く", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle(name(1));
    // 件数の行が total を出すよう、全部に合う検索で絞る。
    await user.type(screen.getByRole("searchbox", { name: "Search tags" }), "Tag");
    await waitFor(() => expect(count()).toBe("150 of 150 tags"));
    expect(screen.queryByTitle(name(140))).toBeNull();

    await user.click(within(rowOf(name(1))).getByRole("button", { name: "Synonyms" }));
    const dialog = await screen.findByRole("dialog", {
      name: `Synonyms of "${name(1)}"`,
    });
    await user.type(
      within(dialog).getByRole("textbox", { name: "Add synonym" }),
      `${name(140)}{Enter}`,
    );
    await within(dialog).findByText(/is a tag on/);
    await user.click(within(dialog).getByRole("button", { name: "Merge" }));
    await waitFor(() =>
      expect(server.tags.some((item) => item.name === name(140))).toBe(false),
    );
    await within(dialog).findByText(name(140));
    await user.click(within(dialog).getByRole("button", { name: "Close" }));
    await waitFor(() => expect(count()).toBe("149 of 149 tags"));
  });

  it("新しい検索の先頭のページを待つ間、空の状態は読んだ検索の語のまま出す", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle(name(0));
    const box = screen.getByRole("searchbox", { name: "Search tags" });
    await user.type(box, "zzz");
    expect(await screen.findByText('No tags match "zzz"')).toBeDefined();

    holdGetsFrom = server.getCalls + 1;
    await user.type(box, "q");
    await waitFor(() => expect(heldGetReleases).toHaveLength(1));
    expect(screen.getByText('No tags match "zzz"')).toBeDefined();
    expect(screen.queryByText('No tags match "zzzq"')).toBeNull();

    heldGetReleases[0]!();
    expect(await screen.findByText('No tags match "zzzq"')).toBeDefined();
  });

  it("読み込んでいない統合先へ統合すると、並び順の位置が範囲の中なら差し込み、共有の保持の取り直しが失敗しても統合元と統合先が残らず消えたりしない（R-12）", async () => {
    const user = userEvent.setup();
    // 本数の多い順で Tag 0（5 本）が先頭、残りは 1 本で名前の順。Tag 140 は読み込んでいない。
    server.tags = server.tags.map((item, index) => ({
      ...item,
      tentative: false,
      videoCount: index === 0 ? 5 : 1,
    }));
    install();
    // 共有の保持の購読者がいて、統合のあとの全件の取り直しは失敗する。
    server.failFullGets = true;
    const unsubscribe = subscribeTags(() => undefined);
    try {
      renderPage();
      await screen.findByTitle(name(0));
      await user.click(screen.getByRole("button", { name: "Sort by: Name" }));
      await user.click(await screen.findByRole("menuitemradio", { name: "Video count" }));
      await waitFor(() =>
        expect(server.pageRequests.at(-1)?.get("sort")).toBe("countDesc"),
      );
      await waitFor(() => expect(loadedNames()[1]).toBe(name(1)));
      expect(screen.queryByTitle(name(140))).toBeNull();

      await user.click(
        within(rowOf(name(0))).getByRole("button", { name: "More actions" }),
      );
      await user.click(
        await screen.findByRole("menuitem", { name: "Merge into another tag…" }),
      );
      const dialog = await screen.findByRole("dialog", { name: `Merge "${name(0)}"` });
      await user.type(
        within(dialog).getByRole("combobox", { name: "Tag to merge into" }),
        name(140),
      );
      await user.click(
        await within(dialog).findByRole("option", { name: new RegExp(`^${name(140)}`) }),
      );
      await user.click(within(dialog).getByRole("button", { name: "Merge" }));

      expect(
        await screen.findByText(`Merged "${name(0)}" into "${name(140)}"`),
      ).toBeDefined();
      await waitFor(() => expect(server.fullGetCalls).toBeGreaterThan(0));
      // 統合先は 6 本で先頭に入り、統合元は消える。件数は 1 つ減る。
      await waitFor(() => expect(loadedNames()[0]).toBe(name(140)));
      expect(screen.queryByTitle(name(0))).toBeNull();
      expect(count()).toBe("149 tags");
      await waitFor(() =>
        expect(document.activeElement).toBe(
          screen.getByRole("link", { name: `Open the library filtered by ${name(140)}` }),
        ),
      );

      // 続きを末尾まで読んでも、統合先は二重に出ない。
      await scrollToEnd(100);
      await waitFor(() => expect(count()).toBe("149 tags"));
      scrollTo(0);
      await waitFor(() => expect(loadedNames()[0]).toBe(name(140)));
      expect(screen.getAllByTitle(name(140))).toHaveLength(1);
    } finally {
      unsubscribe();
    }
  });
});

describe("TagsPage 却下した名前のページ（specs/036-tag-admin-scale/research.md R-13）", () => {
  /** 見張っている番兵（IntersectionObserver）。窓の中身を末尾までスクロールしたことにする。 */
  const observers = new Set<{
    callback: IntersectionObserverCallback;
    target: Element | null;
  }>();

  function scrollRejectedToEnd(skip: ReadonlySet<object> = new Set()) {
    act(() => {
      for (const observer of [...observers]) {
        if (observer.target === null || skip.has(observer)) continue;
        observer.callback(
          [
            {
              isIntersecting: true,
              target: observer.target,
            } as IntersectionObserverEntry,
          ],
          {} as IntersectionObserver,
        );
      }
    });
  }

  const names = Array.from(
    { length: 250 },
    (_, index) => `Name${String(index).padStart(3, "0")}`,
  );

  function rowOf(name: string): HTMLElement {
    return screen.getByTitle(name).closest("[data-tag-id]")!;
  }

  function bar(): HTMLElement {
    return screen.getByRole("region", { name: "Selected tags" });
  }

  beforeEach(() => {
    observers.clear();
    vi.stubGlobal(
      "IntersectionObserver",
      class {
        private entry: { callback: IntersectionObserverCallback; target: Element | null };
        constructor(callback: IntersectionObserverCallback) {
          this.entry = { callback, target: null };
        }
        observe(target: Element) {
          this.entry.target = target;
          observers.add(this.entry);
        }
        disconnect() {
          observers.delete(this.entry);
        }
      },
    );
    server.tags = [
      tag({ id: 1, name: "Alpha", tentative: true, videoCount: 2 }),
      tag({ id: 2, name: "Beta", tentative: true }),
    ];
    server.rejectedNames = [...names];
  });

  const openRejected = openRejectedTab;

  /** backToTags は「Tags」のタブへ戻り、行が描かれるのを待つ。 */
  async function backToTags(user: ReturnType<typeof userEvent.setup>) {
    await user.click(tagsTab());
    await screen.findByTitle("Alpha");
  }

  it("開くと先頭の1ページだけを limit=100 で読み、タブに total を出す。タブを開くために読み直さない", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("Alpha");

    const entry = rejectedTab();
    await waitFor(() => expect(entry.textContent).toContain("250"));
    expect(server.rejectedGetRequests).toHaveLength(1);
    expect(server.rejectedGetRequests[0]?.get("limit")).toBe("100");
    expect(server.rejectedGetRequests[0]?.has("cursor")).toBe(false);

    const list = await openRejected(user);
    expect(within(list).getAllByRole("row")).toHaveLength(100);
    expect(server.rejectedGetRequests).toHaveLength(1);
  });

  it("タブの並びを末尾までスクロールすると cursor 付きで続きを読み、名前を足す。タブを離れても続きは残る", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("Alpha");
    const list = await openRejected(user);

    scrollRejectedToEnd();
    await waitFor(() => expect(within(list).getAllByRole("row")).toHaveLength(200));
    expect(server.rejectedGetRequests[1]?.get("cursor")).toBe("Name099");
    expect(server.rejectedGetRequests[1]?.get("limit")).toBe("100");
    expect(list.getAttribute("aria-busy")).toBeNull();

    // 足した行を描いたあと、番兵を見張り直す effect が走るまでは、送っても続きを読まない。
    // 見張り直すまで送り直す（読み込み中は画面が重ねて送らない）。
    await waitFor(() => {
      scrollRejectedToEnd();
      expect(within(list).getAllByRole("row")).toHaveLength(250);
    });
    expect(within(list).getByTitle("Name249")).toBeDefined();
    // 末尾まで読んだら、もう番兵は見張らない。
    expect(observers.size).toBe(0);
    const requests = server.rejectedGetRequests.length;

    await backToTags(user);
    const reopened = await openRejected(user);
    expect(within(reopened).getAllByRole("row")).toHaveLength(250);
    expect(server.rejectedGetRequests).toHaveLength(requests);
  });

  it("×で外すと DELETE を送り、その名前が消えて入口の件数が1減る。一覧は取り直さない", async () => {
    const user = userEvent.setup();
    const fetchMock = install();
    renderPage();
    await screen.findByTitle("Alpha");
    const entry = rejectedTab();
    await waitFor(() => expect(entry.textContent).toContain("250"));
    const list = await openRejected(user);

    await user.click(within(list).getByRole("button", { name: 'Allow "Name000" again' }));
    await waitFor(() => expect(within(list).queryByTitle("Name000")).toBeNull());
    expect(
      fetchMock.mock.calls.some(
        ([input, init]) =>
          init?.method === "DELETE" &&
          String(input).startsWith("/api/tags/rejected-names?"),
      ),
    ).toBe(true);
    expect(entry.textContent).toContain("249");
    expect(within(list).getAllByRole("row")).toHaveLength(99);
    expect(server.rejectedGetRequests).toHaveLength(1);
  });

  it("まとめての却下のあと、先頭の1ページだけを取り直し、窓の並びが先頭のページに戻る", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("Alpha");
    const list = await openRejected(user);
    scrollRejectedToEnd();
    await waitFor(() => expect(within(list).getAllByRole("row")).toHaveLength(200));
    await backToTags(user);

    const before = server.rejectedGetRequests.length;
    await user.click(within(rowOf("Alpha")).getByRole("checkbox"));
    await user.click(within(bar()).getByRole("button", { name: "Reject…" }));
    const dialog = await screen.findByRole("alertdialog", {
      name: "Reject selected tags",
    });
    const reject = within(dialog).getByRole("button", { name: "Reject" });
    await waitFor(() => expect((reject as HTMLButtonElement).disabled).toBe(false));
    await user.click(reject);
    expect(await screen.findByText("Rejected 1 tag")).toBeDefined();

    const entry = rejectedTab();
    await waitFor(() => expect(entry.textContent).toContain("251"));
    const reloads = server.rejectedGetRequests.slice(before);
    expect(reloads).toHaveLength(1);
    expect(reloads[0]?.has("cursor")).toBe(false);
    expect(reloads[0]?.get("limit")).toBe("100");

    const reopened = await openRejected(user);
    expect(within(reopened).getAllByRole("row")).toHaveLength(100);
    expect(within(reopened).getByTitle("Alpha")).toBeDefined();
  });

  it("続きの読み込みに失敗しても読み込んだ名前を残し、Retry で同じカーソルから読み直す", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("Alpha");
    const list = await openRejected(user);

    server.failRejectedMoreGets = true;
    scrollRejectedToEnd();
    const alert = await screen.findByRole("alert");
    expect(alert.textContent).toBe("Couldn't load more rejected namesRetry");
    expect(within(list).getAllByRole("row")).toHaveLength(100);
    // 失敗の間は番兵で読み直さない。
    expect(observers.size).toBe(0);

    server.failRejectedMoreGets = false;
    await user.click(screen.getByRole("button", { name: "Retry" }));
    await waitFor(() => expect(within(list).getAllByRole("row")).toHaveLength(200));
    const more = server.rejectedGetRequests.filter((params) => params.has("cursor"));
    expect(more.map((params) => params.get("cursor"))).toEqual(["Name099", "Name099"]);
    expect(screen.queryByText("Couldn't load more rejected names")).toBeNull();
  });
  it("読み込んだ名前をすべて外しても続きが残っていれば、空の文言ではなく続きの失敗と Retry を残す", async () => {
    const user = userEvent.setup();
    server.rejectedNames = names.slice(0, 101);
    install();
    renderPage();
    await screen.findByTitle("Alpha");
    const list = await openRejected(user);

    server.failRejectedMoreGets = true;
    scrollRejectedToEnd();
    expect((await screen.findByRole("alert")).textContent).toBe(
      "Couldn't load more rejected namesRetry",
    );
    for (const name of names.slice(0, 100)) {
      fireEvent.click(within(list).getByTitle(name).querySelector("button")!);
    }
    await waitFor(() => expect(within(list).queryAllByRole("row")).toHaveLength(0));
    expect(screen.queryByText("No rejected names")).toBeNull();

    server.failRejectedMoreGets = false;
    await user.click(screen.getByRole("button", { name: "Retry" }));
    expect(await within(list).findByTitle("Name100")).toBeDefined();
  });

  it("読み込んだ名前をすべて外しても続きが残っていれば、番兵で続きを読む", async () => {
    const user = userEvent.setup();
    server.rejectedNames = names.slice(0, 101);
    install();
    renderPage();
    await screen.findByTitle("Alpha");
    const list = await openRejected(user);

    for (const name of names.slice(0, 100)) {
      fireEvent.click(within(list).getByTitle(name).querySelector("button")!);
    }
    await waitFor(() => expect(within(list).queryAllByRole("row")).toHaveLength(0));
    expect(screen.queryByText("No rejected names")).toBeNull();

    scrollRejectedToEnd();
    expect(await within(list).findByTitle("Name100")).toBeDefined();
  });

  it("先頭のページの取り直しの間に番兵が見えても、取り直しのあとで見張り直して続きを読む", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("Alpha");
    const list = await openRejected(user);
    await backToTags(user);

    // 却下で先頭のページの取り直しが始まり、その応答は止まる。
    holdRejectedGets = true;
    await user.click(within(rowOf("Beta")).getByRole("button", { name: "More actions" }));
    await user.click(await screen.findByRole("menuitem", { name: "Reject…" }));
    await user.click(await screen.findByRole("button", { name: "Reject" }));
    await waitFor(() => expect(rejectedGetReleases).toHaveLength(1));
    holdRejectedGets = false;

    const entry = rejectedTab();
    const reopened = await openRejected(user);
    // 取り直しの間の通知は無視される。
    scrollRejectedToEnd();
    expect(
      server.rejectedGetRequests.filter((params) => params.has("cursor")),
    ).toHaveLength(0);
    expect(list.isConnected).toBe(false);

    // 取り直しの応答も 100 件で、件数は変わらない。番兵は見えたままなので、今の
    // 見張りにはもう通知が来ない。見張り直したときの最初の通知だけで続きを読む。
    const watching = new Set(observers);
    act(() => rejectedGetReleases.forEach((resolve) => resolve()));
    await waitFor(() => expect(entry.textContent).toContain("251"));
    await waitFor(() =>
      expect([...observers].some((observer) => !watching.has(observer))).toBe(true),
    );
    scrollRejectedToEnd(watching);
    await waitFor(() => expect(within(reopened).getAllByRole("row")).toHaveLength(200));
  });

  it("取り直しと重なった続きの名前の取り外しのあと、取り直して入口の件数を合わせる", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("Alpha");
    const entry = rejectedTab();
    const list = await openRejected(user);
    scrollRejectedToEnd();
    await waitFor(() => expect(within(list).getAllByRole("row")).toHaveLength(200));
    await backToTags(user);

    // 却下で先頭のページの取り直しが始まり、その応答（取り外しの前の 251 件）は止まる。
    holdRejectedGets = true;
    await user.click(within(rowOf("Beta")).getByRole("button", { name: "More actions" }));
    await user.click(await screen.findByRole("menuitem", { name: "Reject…" }));
    await user.click(await screen.findByRole("button", { name: "Reject" }));
    await waitFor(() => expect(rejectedGetReleases).toHaveLength(1));
    holdRejectedGets = false;

    // 古い続きに残る名前を外す。DELETE の応答も止める。
    const reopened = await openRejected(user);
    holdRejectedDeletes = true;
    await user.click(
      within(reopened).getByRole("button", { name: 'Allow "Name150" again' }),
    );
    await waitFor(() => expect(rejectedDeleteReleases).toHaveLength(1));
    holdRejectedDeletes = false;

    // 取り直しの応答が DELETE の応答より先に届く。
    act(() => rejectedGetReleases.forEach((resolve) => resolve()));
    await waitFor(() => expect(entry.textContent).toContain("251"));
    const before = server.rejectedGetRequests.length;

    act(() => rejectedDeleteReleases.forEach((resolve) => resolve()));
    await waitFor(() => expect(entry.textContent).toContain("250"));
    expect(server.rejectedGetRequests.length).toBe(before + 1);
    expect(server.rejectedGetRequests.at(-1)?.has("cursor")).toBe(false);
  });
});

describe("TagsPage トップバー・見出し・タブ（specs/036-tag-admin-scale/ui-design.md）", () => {
  function rowOf(name: string): HTMLElement {
    return screen.getByTitle(name).closest("[data-tag-id]")!;
  }

  /** chooseSortKind はトップバーの並び順のメニューから種類を選ぶ。 */
  async function chooseSortKind(user: ReturnType<typeof userEvent.setup>, kind: string) {
    await user.click(screen.getByRole("button", { name: /^Sort by: / }));
    await user.click(await screen.findByRole("menuitemradio", { name: kind }));
  }

  beforeEach(() => {
    server.tags = [
      tag({ id: 1, name: "Alpha", tentative: true, videoCount: 2 }),
      tag({ id: 2, name: "Beta", tentative: true }),
      tag({ id: 3, name: "Cat", videoCount: 4 }),
      tag({ id: 4, name: "Gamma" }),
    ];
    latestSearch = "";
  });

  it("検索・Filter・並び順は見出しとタブの下のツールバーに、ライブラリと同じ並び（検索 → Filter → 並び順）で入る", async () => {
    install();
    renderPage({ topBar: true });
    await screen.findByTitle("Alpha");

    // 共通トップバーには置かない（一覧ページのツールバーは本文の見出しの下）。
    expect(within(screen.getByTestId("topbar")).queryByRole("searchbox")).toBeNull();
    const toolbar = document.querySelector<HTMLElement>('[data-slot="toolbar"]')!;
    const tabs = screen.getByRole("tablist", { name: "Tag lists" });
    expect(
      tabs.compareDocumentPosition(toolbar) & Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();
    const topBar = toolbar;
    const search = within(topBar).getByRole("searchbox", { name: "Search tags" });
    expect(search.getAttribute("placeholder")).toBe("Search tags");
    const filter = within(topBar).getByRole("button", { name: "Filter" });
    const sort = within(topBar).getByRole("button", { name: "Sort by: Name" });
    expect(
      search.compareDocumentPosition(filter) & Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();
    expect(
      filter.compareDocumentPosition(sort) & Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();
    // 「Name」に向きは無いので、向きのボタンは出さない。
    expect(within(topBar).queryByRole("button", { name: /Press for/ })).toBeNull();
    expect(screen.getAllByRole("searchbox")).toHaveLength(1);

    // 「/」で検索欄へ移る。
    const user = userEvent.setup();
    await user.keyboard("/");
    expect(document.activeElement).toBe(search);
  });

  it("Filter の吹き出しは「Tentative only」「Unused only」のチェックを持ち、効いている数をボタンに添え、チップと「Clear filters」で外せる", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("Alpha");

    await user.click(filterButton());
    const tentative = await screen.findByRole("checkbox", { name: /^Tentative only/ });
    const unused = screen.getByRole("checkbox", { name: /^Unused only/ });
    expect(tentative.getAttribute("aria-checked")).toBe("false");
    expect(unused.getAttribute("aria-checked")).toBe("false");
    // 何も効いていない間は「Clear filters」を出さない。
    expect(screen.queryByRole("button", { name: "Clear filters" })).toBeNull();

    await user.click(tentative);
    await user.click(unused);
    expect(filterButton().getAttribute("aria-label")).toBe("Filter (2 applied)");
    // 効いている数は soft の Badge で添える。
    expect(filterButton().querySelector('[data-slot="badge"]')?.className).toContain(
      "bg-primary-soft",
    );
    expect(filterButton().textContent).toContain("2");
    expect(await screen.findByText("1 of 4 tags")).toBeDefined();
    await user.keyboard("{Escape}");

    // 見出しの下に、効いている絞り込みのチップが並ぶ。
    const chips = screen.getByRole("list", { name: "Active filters" });
    expect(
      within(chips)
        .getAllByRole("button")
        .map((chip) => chip.textContent),
    ).toEqual(["Tentative only", "Unused only"]);

    // チップの × でその絞り込みだけを外し、残るチップへフォーカスが移る。
    await user.click(
      within(chips).getByRole("button", { name: 'Remove the filter "Tentative only"' }),
    );
    expect(await screen.findByText("2 of 4 tags")).toBeDefined();
    await waitFor(() =>
      expect(document.activeElement).toBe(
        screen.getByRole("button", { name: 'Remove the filter "Unused only"' }),
      ),
    );
    expect(filterButton().getAttribute("aria-label")).toBe("Filter (1 applied)");

    // 「Clear filters」で残りを外す。
    await user.click(filterButton());
    await user.click(await screen.findByRole("button", { name: "Clear filters" }));
    expect(await screen.findByText("4 tags")).toBeDefined();
    expect(screen.queryByRole("list", { name: "Active filters" })).toBeNull();
    expect(filterButton().getAttribute("aria-label")).toBe("Filter");
  });

  it("最後のチップを外すと「Filter」へフォーカスが移る", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("Alpha");
    await toggleFilter(user, "Unused only");

    await user.click(
      screen.getByRole("button", { name: 'Remove the filter "Unused only"' }),
    );
    expect(await screen.findByText("4 tags")).toBeDefined();
    await waitFor(() => expect(document.activeElement).toBe(filterButton()));
  });

  it("見出しは「Tags」で、右に件数と「New tag」を置く。絞ると「N of M tags」で、ページの区切りは出さない", async () => {
    const user = userEvent.setup();
    server.tags = Array.from({ length: 150 }, (_, index) =>
      tag({ id: index + 1, name: `Tag ${String(index).padStart(3, "0")}` }),
    );
    install();
    renderPage();
    await screen.findByText("150 tags");

    const heading = screen.getByRole("heading", { level: 1, name: "Tags" });
    const header = heading.closest("header")!;
    const status = within(header).getByRole("status");
    // 150 件のうち 100 件だけを読んだ状態でも、全部の数だけを出す。
    expect(server.pageRequests.at(-1)?.get("limit")).toBe("100");
    expect(status.textContent).toBe("150 tags");
    expect(within(header).getByRole("button", { name: "New tag" })).toBeDefined();
    expect(document.body.textContent).not.toMatch(/loaded/);

    await user.type(screen.getByRole("searchbox", { name: "Search tags" }), "Tag 01");
    await waitFor(() => expect(status.textContent).toBe("10 of 150 tags"));
    // 150 件を描き、打鍵ごとに引き直すので、負荷の高い `task check` の中では既定の 5 秒を
    // 超えることがある。表明は弱めず、時間だけを仕事の量に合わせる。
  }, 20_000);

  it("列の見出しは先頭のチェック・「Name」・右寄せの「Videos」で、表の見出しの行にある", async () => {
    install();
    renderPage();
    await screen.findByTitle("Alpha");

    const selectAll = screen.getByRole("checkbox", { name: "Select all 4 loaded tags" });
    const columns = selectAll.closest("thead")!;
    expect(
      within(columns)
        .getAllByRole("columnheader")
        .map((cell) => cell.textContent),
    ).toEqual(["", "Name", "Videos"]);
    const videos = within(columns).getByRole("columnheader", { name: "Videos" });
    expect(videos.className).toContain("text-right");
    // 列の見出しは最初の行より前にある。
    expect(
      columns.compareDocumentPosition(rowOf("Alpha")) & Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();
  });

  it("タブで「Rejected names」へ移ると、ツールバーと見出しの件数・「New tag」を外し、本文に却下した名前を出す。URL に tab が載る", async () => {
    const user = userEvent.setup();
    server.rejectedNames = ["Old", "Stale"];
    install();
    renderPage({ topBar: true });
    await screen.findByTitle("Alpha");
    await waitFor(() => expect(rejectedTab().textContent).toBe("Rejected names2"));
    expect(tagsTab().textContent).toBe("Tags4");
    expect(screen.getByRole("tablist", { name: "Tag lists" })).toBeDefined();

    await user.click(rejectedTab());
    expect(rejectedTab().getAttribute("aria-selected")).toBe("true");
    expect(tagsTab().getAttribute("aria-selected")).toBe("false");
    expect(latestSearch).toContain("tab=rejected");
    const panel = screen.getByRole("tabpanel", { name: /Rejected names/ });
    expect(
      within(panel).getAllByRole("button", { name: /^Allow ".*" again$/ }),
    ).toHaveLength(2);
    expect(within(panel).getAllByRole("row")[0]!.textContent).toContain("Allow again");
    expect(screen.queryByRole("searchbox")).toBeNull();
    expect(within(screen.getByTestId("topbar")).queryByRole("button")).toBeNull();
    expect(screen.queryByRole("button", { name: "New tag" })).toBeNull();
    expect(
      screen.queryByRole("link", { name: "Open the library filtered by Alpha" }),
    ).toBeNull();

    // 左右の矢印でタブの間を動く。
    rejectedTab().focus();
    await user.keyboard("{ArrowLeft}");
    expect(document.activeElement).toBe(tagsTab());
    expect(tagsTab().getAttribute("aria-selected")).toBe("true");
    expect(await screen.findByTitle("Alpha")).toBeDefined();
    expect(latestSearch).not.toContain("tab=");
  });

  it("作成の送信中に戻るで「Rejected names」へ移っても「Tags」に留まり、失敗すれば下書きと誤りを残して URL を「Tags」へ戻す", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("Alpha");
    await user.click(rejectedTab());
    await screen.findByRole("tabpanel", { name: /Rejected names/ });
    await user.click(tagsTab());
    await screen.findByTitle("Alpha");

    await user.click(screen.getByRole("button", { name: "New tag" }));
    const input = screen.getByRole("textbox", { name: "New tag name" });
    await user.type(input, "Alpha");
    holdNextMutation = true;
    await user.keyboard("{Enter}");
    await waitFor(() => expect(input.getAttribute("aria-busy")).toBe("true"));

    await historyBack();
    expect(latestSearch).toContain("tab=rejected");
    expect(tagsTab().getAttribute("aria-selected")).toBe("true");
    expect(screen.getByRole("textbox", { name: "New tag name" })).toBe(input);

    release?.();
    expect(await screen.findByText('A tag named "Alpha" already exists.')).toBeDefined();
    expect((input as HTMLInputElement).value).toBe("Alpha");
    await waitFor(() => expect(latestSearch).not.toContain("tab="));
    expect(tagsTab().getAttribute("aria-selected")).toBe("true");
    expect(screen.queryByRole("tabpanel", { name: /Rejected names/ })).toBeNull();
  });

  it("改名の送信中に戻るで「Rejected names」へ移っても「Tags」に留まり、成功すれば「Rejected names」へ移る", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("Alpha");
    await user.click(rejectedTab());
    await screen.findByRole("tabpanel", { name: /Rejected names/ });
    await user.click(tagsTab());
    await screen.findByTitle("Alpha");

    await user.click(within(rowOf("Alpha")).getByRole("button", { name: "Rename" }));
    const input = await screen.findByRole("textbox", { name: 'New name for "Alpha"' });
    await user.clear(input);
    await user.type(input, "Trip");
    holdNextMutation = true;
    await user.keyboard("{Enter}");
    await waitFor(() => expect(input.getAttribute("aria-busy")).toBe("true"));

    await historyBack();
    expect(latestSearch).toContain("tab=rejected");
    expect(tagsTab().getAttribute("aria-selected")).toBe("true");
    expect(screen.getByRole("textbox", { name: 'New name for "Alpha"' })).toBe(input);

    release?.();
    expect(await screen.findByRole("tabpanel", { name: /Rejected names/ })).toBeDefined();
    expect(rejectedTab().getAttribute("aria-selected")).toBe("true");
    expect(latestSearch).toContain("tab=rejected");
  });

  it("作成の送信中は矢印でタブを切り替えず、フォーカスも今のタブに残す", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("Alpha");

    await user.click(screen.getByRole("button", { name: "New tag" }));
    await user.type(screen.getByRole("textbox", { name: "New tag name" }), "新規");
    holdNextMutation = true;
    await user.keyboard("{Enter}");
    await waitFor(() =>
      expect(
        screen.getByRole("textbox", { name: "New tag name" }).getAttribute("aria-busy"),
      ).toBe("true"),
    );

    tagsTab().focus();
    await user.keyboard("{ArrowRight}");
    expect(document.activeElement).toBe(tagsTab());
    expect(tagsTab().getAttribute("aria-selected")).toBe("true");
    await user.click(rejectedTab());
    expect(tagsTab().getAttribute("aria-selected")).toBe("true");
    expect(latestSearch).not.toContain("tab=");

    release?.();
    await waitFor(() =>
      expect(screen.queryByRole("textbox", { name: "New tag name" })).toBeNull(),
    );
    tagsTab().focus();
    await user.keyboard("{ArrowRight}");
    expect(document.activeElement).toBe(rejectedTab());
    await waitFor(() => expect(rejectedTab().getAttribute("aria-selected")).toBe("true"));
  });

  it("タブを移ると選択を解く", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("Alpha");

    await user.click(within(rowOf("Alpha")).getByRole("checkbox"));
    expect(screen.getByRole("region", { name: "Selected tags" })).toBeDefined();
    await user.click(rejectedTab());
    expect(screen.queryByRole("region", { name: "Selected tags" })).toBeNull();
    await user.click(tagsTab());
    await screen.findByTitle("Alpha");
    expect(screen.queryByRole("region", { name: "Selected tags" })).toBeNull();
  });

  it("検索語・絞り込み・並び順・タブは URL に載り、その URL で開き直すと同じ条件で読む", async () => {
    const user = userEvent.setup();
    install();
    const view = renderPage();
    await screen.findByTitle("Alpha");

    await toggleFilter(user, "Tentative only");
    await chooseSortKind(user, "Video count");
    await user.type(screen.getByRole("searchbox", { name: "Search tags" }), "a");
    await waitFor(() => expect(latestSearch).toContain("q=a"));
    const params = new URLSearchParams(latestSearch);
    expect(params.get("tentative")).toBe("1");
    expect(params.get("sort")).toBe("countDesc");
    view.unmount();

    // 開き直す（再読み込み）。
    server.pageRequests = [];
    renderPage({ url: `/tags${latestSearch}&tab=rejected` });
    expect(await screen.findByRole("tabpanel", { name: /Rejected names/ })).toBeDefined();
    await waitFor(() => expect(server.pageRequests.length).toBeGreaterThan(0));
    const sent = server.pageRequests.at(-1)!;
    expect(sent.get("q")).toBe("a");
    expect(sent.get("tentative")).toBe("true");
    expect(sent.get("sort")).toBe("countDesc");

    await user.click(tagsTab());
    expect(
      (screen.getByRole("searchbox", { name: "Search tags" }) as HTMLInputElement).value,
    ).toBe("a");
    expect(screen.getByRole("button", { name: "Sort by: Video count" })).toBeDefined();
    expect(filterButton().getAttribute("aria-label")).toBe("Filter (1 applied)");
    expect(
      screen.getByRole("button", { name: 'Remove the filter "Tentative only"' }),
    ).toBeDefined();
  });
});
