import { act, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {
  MemoryRouter,
  type NavigateFunction,
  useLocation,
  useNavigate,
} from "react-router";
import { afterEach, beforeEach, expect, vi } from "vitest";

import { type TagKind, __resetTagKindsForTest } from "../api/tagKinds";
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

/** Use server-shaped name conflicts but an undisplayable message to verify catalog copy wins. */
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
    kind: null,
    synonyms: [],
    videoCount: 0,
    tentative: false,
    createdAt: "2026-01-01T00:00:00Z",
    ...overrides,
  };
}

/** Apply filters and sorting before keyset pagination; rows moved before the cursor are not returned. */
export function pageOf(tags: readonly Tag[], params: URLSearchParams) {
  const q = foldForMatch(params.get("q") ?? "").trim();
  const sort = (params.get("sort") ?? "name") as TagSort;
  const matched = tags
    .filter((item) => params.get("tentative") !== "true" || item.tentative)
    .filter((item) => params.get("unused") !== "true" || item.videoCount === 0)
    .filter((item) => {
      const kind = params.get("kind");
      if (kind === null) return true;
      if (kind === "none") return item.kind === null;
      return item.kind?.id === Number(kind);
    })
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

export function kind(
  overrides: Partial<TagKind> & { id: number; name: string },
): TagKind {
  return { tagCount: 0, createdAt: "2026-01-01T00:00:00Z", ...overrides };
}

export function presetKinds(): TagKind[] {
  return [
    kind({ id: 1, name: "Genre" }),
    kind({ id: 2, name: "Work" }),
    kind({ id: 3, name: "Place" }),
    kind({ id: 4, name: "Character" }),
  ];
}

export const server = {
  tags: [] as Tag[],
  nextId: 100,
  getCalls: 0,
  pageRequests: [] as URLSearchParams[],
  fullGetCalls: 0,
  failFullGets: false,
  /** Override the requested page limit. */
  pageLimitOverride: null as number | null,
  failNextGet: false,
  /** Fail all subsequent reads so background refresh cannot overwrite the local mutation under test. */
  failAllGets: false,
  failNextCreate: false,
  failNextPatch: false,
  failNextMerge: false,
  failNextSynonymsPost: false,
  /**
   * Fail once with tag_merge_required regardless of current ownership to simulate a stale
   * confirmation without timing races.
   */
  forceMergeRequiredOnce: false,
  /** Fail the next mutation once, then reset the override. */
  nextError: null as { status: number; body: Record<string, unknown> } | null,
  rejectedNames: [] as string[],
  failRejectedGets: false,
  failRejectedMoreGets: false,
  rejectedGetRequests: [] as URLSearchParams[],
  confirmCalls: 0,
  batchCalls: [] as { action: string; ids: number[] }[],
  impactCalls: [] as { action: string; ids: number[] }[],
  /** Override the deduplicated impact count. */
  impactVideoCount: null as number | null,
  failImpact: false,
  failNextBatch: false,
  /** Return kinds in server ID order. */
  tagKinds: [] as TagKind[],
  tagKindGets: 0,
  failTagKindGets: false,
  failNextKindDelete: false,
};

/** Deleting a kind preserves its tags without a kind; unrelated routes return undefined. */
function tagKindRoute(
  path: string,
  method: string,
  init: RequestInit | undefined,
): Response | undefined {
  if (path === "/api/tag-kinds" && method === "GET") {
    server.tagKindGets += 1;
    if (server.failTagKindGets) {
      return jsonResponse({ code: "internal", message: "failed" }, 500);
    }
    const items = [...server.tagKinds]
      .sort((a, b) => a.id - b.id)
      .map((item) => ({
        ...item,
        tagCount: server.tags.filter((t) => t.kind?.id === item.id).length,
      }));
    return jsonResponse({ items });
  }
  const notFound = () =>
    jsonResponse(
      { code: "not_found", reason: "tag_kind_not_found", message: "server-side message" },
      404,
    );
  const taken = (name: string) =>
    jsonResponse(
      {
        code: "conflict",
        reason: "tag_kind_name_taken",
        tagName: name,
        message: "server-side message",
      },
      409,
    );
  if (path === "/api/tag-kinds" && method === "POST") {
    const body = JSON.parse(String(init?.body)) as { name: string };
    const conflict = server.tagKinds.find((item) => item.name === body.name);
    if (conflict !== undefined) return taken(conflict.name);
    const created = kind({ id: server.nextId, name: body.name });
    server.nextId += 1;
    server.tagKinds.push(created);
    return jsonResponse(created, 201);
  }
  const match = /^\/api\/tag-kinds\/(\d+)$/.exec(path);
  if (match === null) return undefined;
  const id = Number(match[1]);
  const found = server.tagKinds.find((item) => item.id === id);
  if (found === undefined) return notFound();
  if (method === "PATCH") {
    const body = JSON.parse(String(init?.body)) as { name: string };
    const conflict = server.tagKinds.find(
      (item) => item.id !== id && item.name === body.name,
    );
    if (conflict !== undefined) return taken(conflict.name);
    found.name = body.name;
    for (const item of server.tags) {
      if (item.kind?.id === id) item.kind = { id, name: body.name };
    }
    return jsonResponse(found);
  }
  if (method === "DELETE") {
    if (server.failNextKindDelete) {
      server.failNextKindDelete = false;
      return jsonResponse({ code: "internal", message: "failed" }, 500);
    }
    server.tagKinds = server.tagKinds.filter((item) => item.id !== id);
    for (const item of server.tags) if (item.kind?.id === id) item.kind = null;
    return jsonResponse(null, 204);
  }
  return undefined;
}

/**
 * Already-matching tags are notApplicableIds; a missing destination kind returns 404 without
 * mutation.
 */
function kindBatch(body: { action: string; ids: number[]; kindId?: number }): Response {
  const target =
    body.action === "setKind"
      ? server.tagKinds.find((item) => item.id === body.kindId)
      : undefined;
  if (body.action === "setKind" && target === undefined) {
    return jsonResponse(
      { code: "not_found", reason: "tag_kind_not_found", message: "server-side message" },
      404,
    );
  }
  const ref = target === undefined ? null : { id: target.id, name: target.name };
  const appliedIds: number[] = [];
  const notFoundIds: number[] = [];
  const notApplicableIds: number[] = [];
  for (const id of body.ids) {
    const found = server.tags.find((t) => t.id === id);
    if (found === undefined) notFoundIds.push(id);
    else if ((found.kind?.id ?? null) === (ref?.id ?? null)) notApplicableIds.push(id);
    else {
      found.kind = ref;
      appliedIds.push(id);
    }
  }
  return jsonResponse({ appliedIds, notFoundIds, notApplicableIds });
}

/** Capture rejected names on request, then gate delivery to simulate a stale pre-removal refresh. */

export const rejectedGetReleases: (() => void)[] = [];

/** Defer both mutation and response until release. */

export const rejectedDeleteReleases: (() => void)[] = [];

export const confirmReleases: (() => void)[] = [];

/** Hold synonym deletions independently so tests can resolve concurrent removals in any order. */

export const synonymDeleteReleases: (() => void)[] = [];

/**
 * Hold GETs from this one-based call onward; capture tags at request time, not release, to model
 * stale snapshots.
 */

export const heldGetReleases: (() => void)[] = [];

/** Export mutable request gates together so each test can control release order. */
export const hold = {
  rejectedGets: false,
  rejectedDeletes: false,
  confirms: false,
  nextMutation: false,
  release: null as (() => void) | null,
  synonymDeletes: false,
  getsFrom: null as number | null,
  kindDeletes: false,
};

/** Delete and respond only when released. */
export const kindDeleteReleases: (() => void)[] = [];

export function install() {
  const fetchMock = vi.fn<typeof fetch>();
  fetchMock.mockImplementation((input, init) => {
    const url = new URL(String(input), "http://localhost");
    const path = url.pathname;
    const method = init?.method ?? "GET";

    if (path.startsWith("/api/tag-kinds")) {
      if (hold.kindDeletes && method === "DELETE") {
        return new Promise((resolve) => {
          kindDeleteReleases.push(() => resolve(tagKindRoute(path, method, init)!));
        });
      }
      const response = tagKindRoute(path, method, init);
      if (response !== undefined) return Promise.resolve(response);
    }

    if (path === "/api/tags" && method === "GET" && !url.searchParams.has("limit")) {
      // Unbounded reads belong to the shared candidates cache, not the management page.
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
      // Snapshot response content at request time.
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
      // Match server keyset pagination after the previous page's final name.
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
      const body = JSON.parse(String(init?.body)) as {
        action: string;
        ids: number[];
        kindId?: number;
      };
      server.batchCalls.push(body);
      if (server.failNextBatch) {
        server.failNextBatch = false;
        return Promise.resolve(
          jsonResponse({ code: "internal", message: "failed" }, 500),
        );
      }
      if (body.action === "setKind" || body.action === "clearKind") {
        return Promise.resolve(kindBatch(body));
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
      // Skip missing sources into notFoundIds; an entirely missing source set leaves the target unchanged.
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
        // Re-registering this tag's synonym is idempotent and returns 200.
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
          // A different tag owns the name; require matching mergeTagId consent.
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

export const probe = { search: "" };

let historyNavigate: NavigateFunction | null = null;

export function LocationProbe() {
  probe.search = useLocation().search;
  historyNavigate = useNavigate();
  return null;
}

export async function historyBack() {
  await act(async () => {
    void historyNavigate!(-1);
    await Promise.resolve();
  });
}

export async function historyForward() {
  await act(async () => {
    void historyNavigate!(1);
    await Promise.resolve();
  });
}

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

export function filterButton(): HTMLElement {
  return screen.getByRole("button", { name: /^Filter/ });
}

export type FilterName = "Tentative only" | "Unused only";

export async function toggleFilter(
  user: ReturnType<typeof userEvent.setup>,
  name: FilterName,
) {
  await user.click(filterButton());
  await user.click(await screen.findByRole("checkbox", { name: new RegExp(`^${name}`) }));
  await user.keyboard("{Escape}");
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
}

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

export function rejectedTab(): HTMLElement {
  return screen.getByRole("tab", { name: /Rejected names/ });
}

export function kindsTab(): HTMLElement {
  return screen.getByRole("tab", { name: /^Kinds/ });
}

export function tagsTab(): HTMLElement {
  return screen.getByRole("tab", { name: /^Tags/ });
}

export async function openRejectedTab(user: ReturnType<typeof userEvent.setup>) {
  await user.click(rejectedTab());
  const panel = await screen.findByRole("tabpanel", { name: /Rejected names/ });
  return within(panel).findByRole("table", { name: "Rejected names" });
}

/** Call once at file scope to reset server and caches per test and restore fetch afterward. */
export function setUpTagsPageServer() {
  beforeEach(() => {
    __resetTagsForTest();
    __resetTagKindsForTest();
    server.tagKinds = presetKinds();
    server.tagKindGets = 0;
    server.failTagKindGets = false;
    server.failNextKindDelete = false;
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
    hold.kindDeletes = false;
    kindDeleteReleases.length = 0;
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
    // Window virtualization scrolls the document; jsdom needs a scrollTo stub.
    vi.spyOn(window, "scrollTo").mockImplementation(() => {});
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });
}
