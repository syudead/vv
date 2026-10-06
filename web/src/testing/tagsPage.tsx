/**
 * tagsPage はタグ管理画面（src/tags/TagsPage.tsx）のテストが共有する偽のサーバーと
 * 操作の手順である。テストは画面の話題ごとのファイル（src/tags/TagsPage*.test.tsx）に
 * 分けてあり、vitest がファイルごとに並べて流す。
 */
import { act, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {
  MemoryRouter,
  type NavigateFunction,
  useLocation,
  useNavigate,
} from "react-router";
import { afterEach, beforeEach, expect, vi } from "vitest";

import { type Tag, type TagSort, __resetTagsForTest } from "../api/tags";
import { foldForMatch } from "../lib/foldForMatch";
import { compareTagsForSort } from "../tags/tagPageRows";
import { ToastProvider } from "../ui/Toast";
import { TooltipProvider } from "../ui/shadcn/tooltip";
import TagsPage from "../tags/TagsPage";

export function jsonResponse(body: unknown, status = 200): Response {
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
export function takenResponse(submitted: string, ownerName: string): Response {
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

export function tag(overrides: Partial<Tag> & { id: number; name: string }): Tag {
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
export function pageOf(tags: readonly Tag[], params: URLSearchParams) {
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
export const server = {
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
 * hold.rejectedGets を true にした間の GET /api/tags/rejected-names は、release を
 * 呼ぶまで応答しない。応答の中身は要求を受けた時点の `server.rejectedNames` を
 * 写し取って持つ（取り外しの前に始まった取り直しの古い応答を再現する）。
 */

export const rejectedGetReleases: (() => void)[] = [];

/**
 * hold.rejectedDeletes を true にした間の DELETE /api/tags/rejected-names は、release を
 * 呼ぶまでサーバーの並びを変えず、応答もしない。
 */

export const rejectedDeleteReleases: (() => void)[] = [];

/** hold.confirms を true にした間の POST /api/tags/{id}/confirm は、release() を呼ぶまで応答しない。 */

export const confirmReleases: (() => void)[] = [];

/** hold.nextMutation を true にした次の POST・PATCH は、release() を呼ぶまで応答しない（B3）。 */

/**
 * hold.synonymDeletes を true にした間の DELETE /api/tags/{id}/synonyms は、
 * それぞれの release を呼ぶまで応答しない。並行する複数の解除を、任意の
 * 順で・同じタイミングで解決させて確かめる（N5・並行する解除）。
 */

export const synonymDeleteReleases: (() => void)[] = [];

/**
 * hold.getsFrom を GET /api/tags の何回目（1始まり）から止めるかに設定すると、
 * それ以降の GET は release を呼ぶまで応答しない。応答の中身は、要求を
 * 受けた時点の `server.tags` を写し取って持つ（release を呼んだ時点の
 * `server.tags` ではない。Devin の指摘4のテストで、追い越された古い取得の
 * 応答が「その要求を送った時点でのサーバーの状態」を持つようにするため）。
 */

export const heldGetReleases: (() => void)[] = [];

/**
 * hold は、テストが応答を止める口である。各項目は止めている間の要求を、対応する
 * release を呼ぶまで応答させない。テストのファイルが書き換えられるように、
 * 1 つの対象にまとめて export する。
 */
export const hold = {
  rejectedGets: false,
  rejectedDeletes: false,
  confirms: false,
  /** true にした次の POST・PATCH は、hold.release() を呼ぶまで応答しない（B3）。 */
  nextMutation: false,
  release: null as (() => void) | null,
  synonymDeletes: false,
  getsFrom: null as number | null,
};

export function install() {
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
      if (hold.getsFrom !== null && server.getCalls >= hold.getsFrom) {
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
      if (hold.rejectedGets) {
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
      if (!hold.nextMutation) return Promise.resolve(respond());
      hold.nextMutation = false;
      return new Promise((resolve) => {
        hold.release = () => resolve(respond());
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
      if (hold.rejectedDeletes) {
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
      if (!hold.confirms) return Promise.resolve(respond());
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
      if (!hold.synonymDeletes) return Promise.resolve(respond());
      return new Promise((resolve) => {
        synonymDeleteReleases.push(() => resolve(respond()));
      });
    }

    throw new Error(`unexpected request: ${method} ${path}`);
  });
  vi.stubGlobal("fetch", fetchMock);
  return fetchMock;
}

/** probe.search は `LocationProbe` が最後に見た URL のクエリである。 */
export const probe = { search: "" };

/** historyNavigate はブラウザの戻る・進む（`navigate(-1)` など）である。 */
let historyNavigate: NavigateFunction | null = null;

/** LocationProbe は今の URL のクエリを `probe.search` に写し、履歴を動かす口を残す。 */
export function LocationProbe() {
  probe.search = useLocation().search;
  historyNavigate = useNavigate();
  return null;
}

/** historyBack はブラウザの「戻る」を押す。 */
export async function historyBack() {
  await act(async () => {
    void historyNavigate!(-1);
    await Promise.resolve();
  });
}

/**
 * renderPage はタグ管理画面を描く。`url` で開く URL（検索語・絞り込み・並び順・タブの
 * クエリ）を、`topBar` で共通トップバーの差し込み先（`TopBarPortal`）を置ける。
 */
export function renderPage({
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
export function filterButton(): HTMLElement {
  return screen.getByRole("button", { name: /^Filter/ });
}

export type FilterName = "Tentative only" | "Unused only";

/**
 * toggleFilter は「Filter」の吹き出しを開いて、その絞り込みのチェックを押し、Esc で
 * 吹き出しを閉じる（フォーカスは「Filter」へ戻る）。
 */
export async function toggleFilter(
  user: ReturnType<typeof userEvent.setup>,
  name: FilterName,
) {
  await user.click(filterButton());
  await user.click(await screen.findByRole("checkbox", { name: new RegExp(`^${name}`) }));
  await user.keyboard("{Escape}");
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
}

/** filterChecked は吹き出しを開いて、その絞り込みのチェックの状態を読み、閉じる。 */
export async function filterChecked(
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
export function rejectedTab(): HTMLElement {
  return screen.getByRole("tab", { name: /Rejected names/ });
}

/** tagsTab は「Tags」のタブである。 */
export function tagsTab(): HTMLElement {
  return screen.getByRole("tab", { name: /^Tags/ });
}

/** openRejectedTab は「Rejected names」のタブへ移り、却下した名前の並びを返す。 */
export async function openRejectedTab(user: ReturnType<typeof userEvent.setup>) {
  await user.click(rejectedTab());
  const panel = await screen.findByRole("tabpanel", { name: /Rejected names/ });
  return within(panel).findByRole("table", { name: "Rejected names" });
}

/**
 * setUpTagsPageServer は各テストの前に偽のサーバーと保持を初期状態へ戻し、後で
 * fetch の差し替えを解く。テストのファイルの先頭で 1 回呼ぶ。
 */
export function setUpTagsPageServer() {
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
    hold.nextMutation = false;
    hold.release = null;
    hold.synonymDeletes = false;
    synonymDeleteReleases.length = 0;
    hold.getsFrom = null;
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
    hold.confirms = false;
    confirmReleases.length = 0;
    hold.rejectedGets = false;
    rejectedGetReleases.length = 0;
    hold.rejectedDeletes = false;
    rejectedDeleteReleases.length = 0;
    // 一覧の仮想化（useWindowVirtualizer）は文書をスクロールする。jsdom には
    // window.scrollTo が無いので、何もしない実装に置き換える。
    vi.spyOn(window, "scrollTo").mockImplementation(() => {});
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });
}
