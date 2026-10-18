import { expect, test, type APIRequestContext, type Page } from "@playwright/test";

// Verify description collapsing and overflow with real layout; lower-level suites cover work-info behavior.
// Use per-attempt person names and clear persisted details before each retry because the server and database survive worker replacement.

interface MediaFolder {
  id: number;
  version: number;
}

interface Video {
  id: number;
  title: string;
}

interface Credit {
  id: number;
  name: string;
}

const origin = "http://127.0.0.1:15173";
const mutationHeaders = { Origin: origin, "Content-Type": "application/json" };

/** Worker replacement reloads this module and gives retries a new marker. */
const run = Date.now().toString(36);

/** Use names distinct from other e2e fixtures. */
const haruka = `作品e2e春香${run}`;

let folder: MediaFolder | undefined;
const videos = new Map<string, Video>();
const people = new Map<string, Credit>();

function video(title: string): Video {
  const found = videos.get(title);
  if (found === undefined) throw new Error(`video not indexed: ${title}`);
  return found;
}

function person(name: string): Credit {
  const found = people.get(name);
  if (found === undefined) throw new Error(`person not known: ${name}`);
  return found;
}

async function runScan(request: APIRequestContext) {
  const scan = await request.post("/api/scans", { headers: mutationHeaders, data: {} });
  expect(scan.status()).toBe(202);
  const scanId = ((await scan.json()) as { id: number }).id;
  await expect
    .poll(
      async () => {
        const response = await request.get("/api/scans/current");
        const current = (await response.json()) as { id: number; state: string };
        return current.id === scanId && current.state !== "running"
          ? current.state
          : "running";
      },
      { timeout: 60_000 },
    )
    .toBe("done");
}

async function createCredit(
  request: APIRequestContext,
  kind: "people" | "organizations",
  data: { name: string },
): Promise<Credit> {
  const response = await request.post(`/api/${kind}`, { headers: mutationHeaders, data });
  expect(response.status()).toBe(201);
  return (await response.json()) as Credit;
}

async function setWorkInfo(request: APIRequestContext, id: number, workInfo: object) {
  const response = await request.put(`/api/videos/${String(id)}/work-info`, {
    headers: mutationHeaders,
    data: workInfo,
  });
  expect(response.ok()).toBe(true);
}

/** Clear persisted details from prior attempts and restore filename-derived titles. */
async function clearDetails(request: APIRequestContext, id: number): Promise<Video> {
  const response = await request.put(`/api/videos/${String(id)}/details`, {
    headers: mutationHeaders,
    data: { displayName: "", workInfo: {} },
  });
  expect(response.ok()).toBe(true);
  return (await response.json()) as Video;
}

function infoArea(page: Page) {
  return page.getByRole("list", { name: "File details" }).locator("xpath=../..");
}

async function openVideo(page: Page, id: number, title: string) {
  await page.goto(`/videos/${String(id)}`);
  await expect(page.getByRole("heading", { level: 1 })).toHaveText(title);
}

test.describe.serial("work information", () => {
  test.beforeAll(async ({ request }) => {
    test.setTimeout(120_000);
    const root = process.env.MDM_E2E_WORK_INFO_MEDIA_DIR;
    if (root === undefined)
      throw new Error("MDM_E2E_WORK_INFO_MEDIA_DIR is not configured");

    const created = await request.post("/api/media-folders", {
      headers: mutationHeaders,
      data: { path: root },
    });
    expect(created.status()).toBe(201);
    folder = (await created.json()) as MediaFolder;
    await runScan(request);

    const list = await request.get("/api/videos?limit=200");
    const page = (await list.json()) as { items: Video[] };
    expect(page.items).toHaveLength(4);
    for (const item of page.items) {
      const cleared = await clearDetails(request, item.id);
      videos.set(cleared.title, cleared);
    }

    people.set(haruka, await createCredit(request, "people", { name: haruka }));
  });

  test.afterAll(async ({ request }) => {
    if (folder === undefined) return;
    const removed = await request.delete(
      `/api/media-folders/${String(folder.id)}?version=${String(folder.version)}`,
      { headers: mutationHeaders },
    );
    expect(removed.status()).toBe(204);
  });

  test("Edge Case: 長い説明は折りたたまれたままで、360px でページに横スクロールが出ない", async ({
    page,
    request,
  }) => {
    const b = video("作品動画B");
    const longName = `作品e2e長い名前${run}${"あ".repeat(120)}`;
    const long = await createCredit(request, "people", { name: longName });
    const paragraph = "長い説明の一文です。".repeat(30);
    const description = [
      paragraph,
      `https://example.com/${"a".repeat(240)}`,
      paragraph,
      paragraph,
    ].join("\n");
    await setWorkInfo(request, b.id, {
      code: `LONG-${"9".repeat(60)}`,
      description,
      people: [
        { role: "performer", person: { id: person(haruka).id } },
        { role: "performer", person: { id: long.id } },
      ],
    });

    await page.setViewportSize({ width: 360, height: 800 });
    await openVideo(page, b.id, b.title);
    const text = infoArea(page).locator("p").filter({ hasText: "長い説明の一文です。" });
    await expect(text).toBeVisible();
    await expect(page.getByRole("button", { name: "Show more" })).toBeVisible();
    expect(await text.evaluate((node) => node.scrollHeight > node.clientHeight)).toBe(
      true,
    );
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= window.innerWidth,
      ),
    ).toBe(true);

    await page.getByRole("button", { name: "Show more" }).click();
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= window.innerWidth,
      ),
    ).toBe(true);
    await page.getByRole("button", { name: "Show less" }).click();
    await expect(page.getByRole("button", { name: "Show more" })).toBeVisible();
  });
});
