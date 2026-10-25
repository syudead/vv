import { createServer, type IncomingMessage, type Server } from "node:http";
import type { AddressInfo } from "node:net";

import {
  expect,
  test,
  type APIRequestContext,
  type Locator,
  type Page,
} from "@playwright/test";

// The assistant against the real server and worker, with a stub Ollama that answers fixed outcomes for fixed
// requests; a real model's reading of Japanese requests is checked by hand (docs/how-to/running-vv.md).

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

const filterText = "未視聴だけ見せて";
const attachText = "これに助手付けタグを付けて";
const mergeText = "助手旧タグを助手新タグに統合して";
const attachTag = "助手付けタグ";
const sourceTag = "助手旧タグ";
const destinationTag = "助手新タグ";
const folderName = "助手の未視聴";

// Each request text maps to the one outcome a model would answer; anything else is unsupported.
const outcomes: Record<string, unknown> = {
  [filterText]: { kind: "filter", filter: { watchState: "unwatched" } },
  [attachText]: {
    kind: "action",
    action: {
      operation: "updateVideoTags.add",
      target: { kind: "selection" },
      arguments: { tagNames: [attachTag] },
    },
  },
  [mergeText]: {
    kind: "action",
    action: {
      operation: "mergeTag",
      target: { kind: "named", names: [destinationTag] },
      arguments: { names: [sourceTag] },
    },
  },
};

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

/** The chat API only: the server saves the connection without contacting Ollama. */
async function startStubOllama(): Promise<Server> {
  const server = createServer((req, res) => {
    void (async () => {
      if (req.method !== "POST" || req.url !== "/api/chat") {
        res.writeHead(404, { "Content-Type": "application/json" });
        res.end(JSON.stringify({ error: "not found" }));
        return;
      }
      const body = JSON.parse(await readBody(req)) as {
        messages: { content: string }[];
      };
      const prompt = body.messages.at(-1)?.content ?? "";
      // The instructions name the mark too, so the request is the last marked text.
      const start = prompt.lastIndexOf("<request>") + "<request>".length;
      const text = prompt.slice(start, prompt.lastIndexOf("</request>"));
      const outcome = outcomes[text] ?? { kind: "unsupported" };
      res.writeHead(200, { "Content-Type": "application/json" });
      res.end(
        JSON.stringify({
          message: { role: "assistant", content: JSON.stringify(outcome) },
          done: true,
        }),
      );
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
  for (const name of [attachTag, sourceTag, destinationTag]) {
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

async function openPanel(page: Page) {
  await page.getByRole("button", { name: "Assistant", exact: true }).click();
  await expect(panel(page)).toBeVisible();
}

async function ask(page: Page, text: string) {
  await panel(page).getByRole("textbox", { name: "Request" }).fill(text);
  await panel(page).getByRole("button", { name: "Send" }).click();
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

  test("an unset generation model shows the guidance", async ({ page, request }) => {
    await saveConnection(request, endpoint, "");
    await page.goto("/");
    await openPanel(page);
    await expect(panel(page).getByText("Choose a model for the assistant")).toBeVisible();
    await expect(
      panel(page).getByRole("link", { name: "Open Connections" }),
    ).toBeVisible();
    await expect(panel(page).getByRole("textbox", { name: "Request" })).toHaveCount(0);
  });

  test("a filter request narrows the library and saves as a smart folder", async ({
    page,
    request,
  }) => {
    await saveConnection(request, endpoint, "stub-chat");
    await page.goto("/");
    await expect(cardTitles(page).filter({ hasText: "助手確認A" })).toBeVisible();
    await openPanel(page);
    await ask(page, filterText);

    await expect(page).toHaveURL(/[?&]watchState=unwatched(?:&|$)/);
    await expect(panel(page).getByText("Filtered the library")).toBeVisible();
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

  test("an attach request changes nothing until Execute, which creates and attaches the tag", async ({
    page,
    request,
  }) => {
    await page.goto("/?watchState=unwatched");
    for (const title of ["助手確認C", "助手確認D"]) {
      await page.getByRole("checkbox", { name: `Select "${title}"` }).check();
    }
    await openPanel(page);
    await expect(panel(page).getByText("Talking about 2 selected videos")).toBeVisible();
    await ask(page, attachText);

    await expect(panel(page).getByRole("button", { name: "Execute" })).toBeVisible();
    await expect(panel(page).getByText("Add tags", { exact: true })).toBeVisible();
    await expect(panel(page).getByText("2 videos", { exact: true })).toBeVisible();
    await expect(panel(page).getByText(`New tag ${attachTag}`)).toBeAttached();
    // The proposal is stored; only Execute creates the tag and attaches it.
    expect(await findTag(request, attachTag)).toBeUndefined();

    await panel(page).getByRole("button", { name: "Execute" }).click();
    await expect(panel(page).getByText("Done · 2 videos changed")).toBeVisible();
    expect(await findTag(request, attachTag)).toBeDefined();
    for (const title of ["助手確認C", "助手確認D"]) {
      const detail = (await (
        await request.get(`/api/videos/${String(video(title).id)}`)
      ).json()) as { tags: { name: string }[] };
      expect(detail.tags.map((tag) => tag.name)).toContain(attachTag);
    }
  });

  test("a merge request marks the card irreversible with the videos that move", async ({
    page,
    request,
  }) => {
    await page.goto("/");
    await openPanel(page);
    await ask(page, mergeText);

    await expect(
      panel(page).getByText(`Merge tags into “${destinationTag}”`),
    ).toBeVisible();
    await expect(panel(page).getByText("Can’t be undone")).toBeVisible();
    await expect(
      panel(page).getByText(`3 videos move to “${destinationTag}”`),
    ).toBeVisible();

    await panel(page).getByRole("button", { name: "Dismiss" }).click();
    await expect(panel(page).getByText("Dismissed · nothing changed")).toBeVisible();
    expect(await findTag(request, sourceTag)).toBeDefined();
  });
});
