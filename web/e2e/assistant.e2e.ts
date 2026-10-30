import { createServer, type IncomingMessage, type Server } from "node:http";
import type { AddressInfo } from "node:net";

import {
  expect,
  test,
  type APIRequestContext,
  type Locator,
  type Page,
} from "@playwright/test";

// The assistant against the real server and workers, with a stub Ollama that answers a scripted
// sequence of tool calls for each fixed request; a real model is checked by hand
// (docs/how-to/running-vv.md#assistant).

interface MediaFolder {
  id: number;
  path: string;
  version: number;
}

interface Video {
  id: number;
  title: string;
}

interface Tag {
  id: number;
  name: string;
}

const origin = "http://127.0.0.1:15173";
const mutationHeaders = { Origin: origin, "Content-Type": "application/json" };

const toolModel = "stub-chat";
const textModel = "stub-text";

const countText = "助手確認の動画は何本？";
const filterText = "未視聴だけ見せて";
const createAttachText = "助手付けタグを作って選択中の動画に付けて";
const twoTagsText = "助手甲タグと助手乙タグを作って";
const mergeText = "助手旧タグを助手新タグに統合して";
const excludedText = "APIトークンを作って";
const loopText = "助手のタグを読み続けて";
const ungroundedText = "助手確認の再生回数は？";
const attachTag = "助手付けタグ";
const firstTag = "助手甲タグ";
const secondTag = "助手乙タグ";
const sourceTag = "助手旧タグ";
const destinationTag = "助手新タグ";
const folderName = "助手の未視聴";

interface Reply {
  calls?: { name: string; arguments: Record<string, unknown> }[];
  text?: string;
}

function call(name: string, args: Record<string, unknown> = {}): Reply {
  return { calls: [{ name, arguments: args }] };
}

// Each request's replies, one per model call; the last one repeats. A tool outside the core set is
// called after find_tools offers it, under the wire name with "." written "__".
const scripts: Record<string, Reply[]> = {
  [countText]: [
    call("listVideos", { query: "助手確認" }),
    { text: "助手確認の動画は4本です。" },
  ],
  [filterText]: [
    call("show_in_library", { watchState: "unwatched" }),
    { text: "未視聴の動画を表示しました。" },
  ],
  [createAttachText]: [
    {
      calls: [
        { name: "find_tools", arguments: { query: "create tag" } },
        { name: "find_tools", arguments: { query: "add tags to videos" } },
      ],
    },
    call("createTag", { name: attachTag }),
    call("updateVideoTags__add", {
      target: { kind: "selection" },
      tagNames: [attachTag],
    }),
    { text: "タグを作って、選択中の動画に付ける手順を提案しました。" },
  ],
  [twoTagsText]: [
    call("find_tools", { query: "create tag" }),
    {
      calls: [
        { name: "createTag", arguments: { name: firstTag } },
        { name: "createTag", arguments: { name: secondTag } },
      ],
    },
    { text: "タグを作る手順を提案しました。" },
  ],
  [mergeText]: [
    call("find_tools", { query: "merge tags" }),
    call("mergeTag", {
      target: { kind: "named", names: [destinationTag] },
      names: [sourceTag],
    }),
    { text: "タグを統合する手順を提案しました。" },
  ],
  [excludedText]: [call("createApiToken"), { text: "ここではできません。" }],
  [loopText]: [call("listTags")],
  // 99 is in no tool result, so the grounding check refuses the answer twice.
  [ungroundedText]: [call("listVideos"), { text: "再生回数は99回です。" }],
};

/** A request whose last reply waits until the test releases it, so the running turn can be seen. */
const holds = new Map<string, Promise<void>>();

function hold(text: string): () => void {
  let release = () => {};
  holds.set(
    text,
    new Promise<void>((resolve) => {
      release = resolve;
    }),
  );
  return () => {
    holds.delete(text);
    release();
  };
}

function mediaRoot(): string {
  const root = process.env.MDM_E2E_ASSISTANT_MEDIA_DIR;
  if (root === undefined)
    throw new Error("MDM_E2E_ASSISTANT_MEDIA_DIR is not configured");
  return root;
}

async function readBody(req: IncomingMessage): Promise<string> {
  const chunks: Buffer[] = [];
  for await (const chunk of req) chunks.push(chunk as Buffer);
  return Buffer.concat(chunks).toString("utf8");
}

interface ChatBody {
  messages: { role: string; content: string }[];
}

/** The request is the first user message, and the assistant messages so far count the replies. */
async function scriptedReply(body: ChatBody): Promise<unknown> {
  const first = body.messages.find((message) => message.role === "user")?.content ?? "";
  const request = first.replace(/^<request>/, "").replace(/<\/request>$/, "");
  const script = scripts[request] ?? [{ text: "わかりません。" }];
  const step = body.messages.filter((message) => message.role === "assistant").length;
  const reply = script[Math.min(step, script.length - 1)];
  if (step === script.length - 1) await holds.get(request);
  return {
    role: "assistant",
    content: reply.text ?? "",
    tool_calls: reply.calls?.map((item) => ({
      function: { name: item.name, arguments: item.arguments },
    })),
  };
}

/** The chat API, and /api/show, which lists tools for every model but the text-only one. */
async function startStubOllama(): Promise<Server> {
  const server = createServer((req, res) => {
    void (async () => {
      const send = (status: number, value: unknown) => {
        res.writeHead(status, { "Content-Type": "application/json" });
        res.end(JSON.stringify(value));
      };
      if (req.method === "POST" && req.url === "/api/show") {
        const { model } = JSON.parse(await readBody(req)) as { model: string };
        send(200, {
          capabilities: model === textModel ? ["completion"] : ["completion", "tools"],
        });
        return;
      }
      if (req.method !== "POST" || req.url !== "/api/chat") {
        send(404, { error: "not found" });
        return;
      }
      const message = await scriptedReply(JSON.parse(await readBody(req)) as ChatBody);
      send(200, { message, done: true });
    })();
  });
  await new Promise<void>((resolve) => server.listen(0, "127.0.0.1", resolve));
  return server;
}

async function saveConnection(
  request: APIRequestContext,
  endpoint: string,
  generationModel: string,
) {
  const response = await request.put("/api/settings/ollama", {
    headers: mutationHeaders,
    data: { endpoint, model: "clef-flash", generationModel },
  });
  expect(response.status()).toBe(200);
}

async function removeLeftovers(request: APIRequestContext) {
  const folders = (await (
    await request.get("/api/media-folders")
  ).json()) as MediaFolder[];
  for (const folder of folders.filter((item) => item.path === mediaRoot())) {
    const removed = await request.delete(
      `/api/media-folders/${String(folder.id)}?version=${String(folder.version)}`,
      { headers: mutationHeaders },
    );
    expect(removed.status()).toBe(204);
  }
  for (const name of [attachTag, firstTag, secondTag, sourceTag, destinationTag]) {
    const tag = await findTag(request, name);
    if (tag === undefined) continue;
    const removed = await request.delete(`/api/tags/${String(tag.id)}`, {
      headers: mutationHeaders,
    });
    expect(removed.ok()).toBe(true);
  }
  const listed = (await (await request.get("/api/smart-folders")).json()) as {
    smartFolders: Tag[];
  };
  for (const folder of listed.smartFolders.filter((item) => item.name === folderName)) {
    const removed = await request.delete(`/api/smart-folders/${String(folder.id)}`, {
      headers: mutationHeaders,
    });
    expect(removed.status()).toBe(204);
  }
}

async function findTag(
  request: APIRequestContext,
  name: string,
): Promise<Tag | undefined> {
  const response = await request.get(`/api/tags?q=${encodeURIComponent(name)}`);
  expect(response.status()).toBe(200);
  const list = (await response.json()) as { items: Tag[] };
  return list.items.find((tag) => tag.name === name);
}

async function createTag(
  request: APIRequestContext,
  name: string,
  videoIds: number[],
): Promise<void> {
  const created = await request.post("/api/tags", {
    headers: mutationHeaders,
    data: { name },
  });
  expect(created.status()).toBe(201);
  const tag = (await created.json()) as Tag;
  const attached = await request.post("/api/video-tags", {
    headers: mutationHeaders,
    data: { videoIds, action: "add", tag: { id: tag.id } },
  });
  expect(attached.ok()).toBe(true);
}

function panel(page: Page): Locator {
  return page.getByRole("complementary", { name: "Assistant" });
}

/** The trigger's name is "Assistant · working" while a turn runs. */
async function openPanel(page: Page, trigger = "Assistant") {
  await page.getByRole("button", { name: trigger, exact: true }).click();
  await expect(panel(page)).toBeVisible();
}

async function ask(page: Page, text: string) {
  await panel(page).getByRole("textbox", { name: "Request" }).fill(text);
  await panel(page).getByRole("button", { name: "Send" }).click();
}

/** The proposal card that names text; the thread keeps the cards of earlier tests. */
function card(page: Page, text: string): Locator {
  return panel(page).getByRole("article").filter({ hasText: text });
}

function cardTitles(page: Page): Locator {
  return page.locator("article[data-video-id] h3");
}

test.describe.serial("assistant", () => {
  let stub: Server | undefined;
  let endpoint = "";
  const videos = new Map<string, Video>();

  function video(title: string): Video {
    const found = videos.get(title);
    if (found === undefined) throw new Error(`video not indexed: ${title}`);
    return found;
  }

  test.beforeAll(async ({ request }) => {
    test.setTimeout(120_000);
    // A CI retry starts on the data the failed attempt left.
    await removeLeftovers(request);
    stub = await startStubOllama();
    endpoint = `http://127.0.0.1:${String((stub.address() as AddressInfo).port)}`;

    const created = await request.post("/api/media-folders", {
      headers: mutationHeaders,
      data: { path: mediaRoot() },
    });
    expect(created.status()).toBe(201);
    const scan = await request.post("/api/scans", { headers: mutationHeaders, data: {} });
    expect(scan.status()).toBe(202);
    const scanId = ((await scan.json()) as { id: number }).id;
    await expect
      .poll(
        async () => {
          const current = (await (await request.get("/api/scans/current")).json()) as {
            id: number;
            state: string;
          };
          return current.id === scanId ? current.state : "running";
        },
        { timeout: 60_000 },
      )
      .toBe("done");
    const list = (await (await request.get("/api/videos?limit=200")).json()) as {
      items: Video[];
    };
    for (const item of list.items) {
      if (item.title.startsWith("助手確認")) videos.set(item.title, item);
    }
    expect(videos.size).toBe(4);

    // A and B are in progress, so the unwatched filter keeps C and D.
    for (const title of ["助手確認A", "助手確認B"]) {
      const saved = await request.put(`/api/videos/${String(video(title).id)}/progress`, {
        headers: mutationHeaders,
        data: { positionMs: 500 },
      });
      expect(saved.status()).toBe(200);
    }
    await createTag(
      request,
      sourceTag,
      ["助手確認A", "助手確認B", "助手確認C"].map((title) => video(title).id),
    );
    await createTag(request, destinationTag, [video("助手確認D").id]);
  });

  test.afterAll(async ({ request }) => {
    await removeLeftovers(request);
    await new Promise((resolve) => stub?.close(resolve));
  });

  test("a guest sees no assistant trigger", async ({ browser, request }) => {
    const shown = video("助手確認A");
    const visibility = await request.put("/api/video-visibility", {
      headers: mutationHeaders,
      data: { videoIds: [shown.id], public: true },
    });
    expect(visibility.ok()).toBe(true);
    const context = await browser.newContext({
      storageState: { cookies: [], origins: [] },
    });
    try {
      const page = await context.newPage();
      await page.goto("/");
      await expect(cardTitles(page).filter({ hasText: shown.title })).toBeVisible();
      await expect(
        page.getByRole("button", { name: "Assistant", exact: true }),
      ).toHaveCount(0);
      await page.goto(`/videos/${String(shown.id)}`);
      await expect(page.locator("video")).toBeVisible();
      await expect(
        page.getByRole("button", { name: "Assistant", exact: true }),
      ).toHaveCount(0);
    } finally {
      await context.close();
      await request.put("/api/video-visibility", {
        headers: mutationHeaders,
        data: { videoIds: [shown.id], public: false },
      });
    }
  });

  test("a model that can't call tools shows the guidance", async ({ page, request }) => {
    await saveConnection(request, endpoint, textModel);
    await page.goto("/");
    await openPanel(page);
    await expect(panel(page).getByText("The model can’t use tools")).toBeVisible();
    await expect(
      panel(page).getByRole("link", { name: "Open Connections" }),
    ).toBeVisible();
    await expect(panel(page).getByRole("textbox", { name: "Request" })).toHaveCount(0);
  });

  test("progress shows while a request runs, survives closing the panel, and the answer follows", async ({
    page,
    request,
  }) => {
    await saveConnection(request, endpoint, toolModel);
    const release = hold(countText);
    try {
      await page.goto("/");
      await openPanel(page);
      await ask(page, countText);
      await expect(panel(page).getByText("Reading listVideos")).toBeVisible();
      await expect(panel(page).getByText(/^Working · /)).toBeVisible();

      await page.keyboard.press("Escape");
      await expect(panel(page)).toHaveCount(0);
      await openPanel(page, "Assistant · working");
      await expect(panel(page).getByText("Reading listVideos")).toBeVisible();
    } finally {
      release();
    }
    await expect(panel(page).getByText("助手確認の動画は4本です。")).toBeVisible();
    await expect(
      panel(page).getByRole("button", { name: "Show 1 tool call" }),
    ).toBeVisible();
  });

  test("a filter request narrows the library and saves as a smart folder", async ({
    page,
    request,
  }) => {
    await page.goto("/");
    await expect(cardTitles(page).filter({ hasText: "助手確認A" })).toBeVisible();
    await openPanel(page);
    await ask(page, filterText);

    await expect(page).toHaveURL(/[?&]watchState=unwatched(?:&|$)/);
    await expect(cardTitles(page).filter({ hasText: "助手確認C" })).toBeVisible();
    await expect(cardTitles(page).filter({ hasText: "助手確認D" })).toBeVisible();
    await expect(cardTitles(page).filter({ hasText: /助手確認[AB]/ })).toHaveCount(0);

    await page.getByRole("button", { name: "Save as smart folder", exact: true }).click();
    const dialog = page.getByRole("dialog");
    await dialog.getByRole("textbox", { name: "Name" }).fill(folderName);
    await dialog.getByRole("button", { name: "Create" }).click();
    await expect(page.getByText(`Created "${folderName}"`)).toBeVisible();

    const listed = (await (await request.get("/api/smart-folders")).json()) as {
      smartFolders: Tag[];
    };
    const folder = listed.smartFolders.find((item) => item.name === folderName);
    expect(folder).toBeDefined();
    const detail = (await (
      await request.get(`/api/smart-folders/${String(folder?.id)}`)
    ).json()) as { settings: { rule: { watchState?: string } } };
    expect(detail.settings.rule.watchState).toBe("unwatched");
  });

  test("creating a tag and attaching it is one two-step card that Execute runs as one activity entry", async ({
    page,
    request,
  }) => {
    const since = new Date().toISOString();
    await page.goto("/?watchState=unwatched");
    for (const title of ["助手確認C", "助手確認D"]) {
      await page.getByRole("checkbox", { name: `Select "${title}"` }).check();
    }
    await openPanel(page);
    await expect(panel(page).getByText("Talking about 2 selected videos")).toBeVisible();
    await ask(page, createAttachText);

    const steps = card(page, `Create tag “${attachTag}”`);
    await expect(steps.getByText("2 steps · 2 selected")).toBeVisible();
    await expect(steps.getByText("Add tags", { exact: true })).toBeVisible();
    await expect(steps.getByText("Created by step 1")).toBeVisible();
    // The proposal is stored; only Execute creates the tag and attaches it.
    expect(await findTag(request, attachTag)).toBeUndefined();

    await steps.getByRole("button", { name: "Execute 2 steps" }).click();
    await expect(steps.getByText("Done · 2 steps")).toBeVisible();
    expect(await findTag(request, attachTag)).toBeDefined();
    for (const title of ["助手確認C", "助手確認D"]) {
      const detail = (await (
        await request.get(`/api/videos/${String(video(title).id)}`)
      ).json()) as { tags: { name: string }[] };
      expect(detail.tags.map((tag) => tag.name)).toContain(attachTag);
    }

    const entries = (await (
      await request.get(
        `/api/activity?feature=assistant&since=${encodeURIComponent(since)}`,
      )
    ).json()) as { items: { kind: string; detail?: string }[] };
    expect(entries.items).toHaveLength(1);
    expect(entries.items[0]?.kind).toBe("assistant_run");
    const detail = JSON.parse(entries.items[0]?.detail ?? "{}") as {
      steps?: { operation: string }[];
    };
    expect(detail.steps?.map((step) => step.operation)).toEqual([
      "createTag",
      "updateVideoTags.add",
    ]);
  });

  test("an unticked step is skipped and does not run", async ({ page, request }) => {
    await page.goto("/");
    await openPanel(page);
    await ask(page, twoTagsText);

    const steps = card(page, `Create tag “${firstTag}”`);
    await expect(steps.getByText("2 steps · 2 selected")).toBeVisible();
    await steps.getByRole("checkbox", { name: "Include step 2" }).uncheck();
    await expect(steps.getByText("2 steps · 1 selected")).toBeVisible();
    await expect(steps.getByText("Won’t run")).toBeVisible();
    await steps.getByRole("button", { name: "Execute", exact: true }).click();

    await expect(steps.getByText("Done · 1 step")).toBeVisible();
    await expect(steps.getByText("Skipped")).toBeVisible();
    expect(await findTag(request, firstTag)).toBeDefined();
    expect(await findTag(request, secondTag)).toBeUndefined();
  });

  test("a merge request marks the card irreversible with the videos that move", async ({
    page,
    request,
  }) => {
    await page.goto("/");
    await openPanel(page);
    await ask(page, mergeText);

    const merge = card(page, `Merge tags into “${destinationTag}”`);
    await expect(merge.getByText("Can’t be undone")).toBeVisible();
    await expect(merge.getByText(`3 videos move to “${destinationTag}”`)).toBeVisible();

    await merge.getByRole("button", { name: "Dismiss" }).click();
    // A dismissed card gives way to one line.
    await expect(panel(page).getByText("Dismissed · nothing changed")).toBeVisible();
    expect(await findTag(request, sourceTag)).toBeDefined();
  });

  test("an excluded request names its page with a link", async ({ page }) => {
    await page.goto("/");
    await openPanel(page);
    await ask(page, excludedText);

    await expect(
      panel(page).getByText(
        "The assistant can’t do this. Creating API tokens is done on its own page.",
      ),
    ).toBeVisible();
    await expect(panel(page).locator('a[href="/settings/api"]')).toHaveText(
      /^Open Settings, /,
    );
  });

  test("a request stops at the step limit and at an answer no tool supports", async ({
    page,
  }) => {
    await page.goto("/");
    await openPanel(page);
    await ask(page, loopText);
    await expect(
      panel(page).getByText(/^Stopped: the request needed more steps/),
    ).toBeVisible();

    await ask(page, ungroundedText);
    await expect(
      panel(page).getByText("The answer named facts the assistant hadn’t checked", {
        exact: false,
      }),
    ).toBeVisible();
    await expect(panel(page).getByText("再生回数は99回です。")).toHaveCount(0);
  });
});
