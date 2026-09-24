import { createHash } from "node:crypto";
import { mkdir, readFile, readdir } from "node:fs/promises";
import path from "node:path";

import {
  expect,
  test,
  type APIRequestContext,
  type Browser,
  type Page,
  type Request,
} from "@playwright/test";

import { elapse } from "./clock";

interface Video {
  id: number;
  title: string;
  playable: boolean;
  probeState: "pending" | "done" | "failed";
  previewState: "pending" | "done" | "failed";
  previewUrl?: string;
  durationMs?: number;
  progress?: { positionMs: number; completed: boolean; updatedAt: string };
}

interface VideoPage {
  items: Video[];
  total: number;
  nextCursor?: string;
}

interface MediaFolder {
  id: number;
  version: number;
}

const origin = "http://127.0.0.1:15173";
const mutationHeaders = { Origin: origin, "Content-Type": "application/json" };
const screenshotDir = process.env.MDM_E2E_SCREENSHOT_DIR;
const videos = new Map<string, Video>();
let folder: MediaFolder | undefined;
let sourceSnapshot = new Map<string, string>();

function video(title: string): Video {
  const found = videos.get(title);
  if (found === undefined) throw new Error(`video not indexed: ${title}`);
  return found;
}

function card(page: Page, item: Video) {
  return page.locator(`[data-video-id="${String(item.id)}"]`);
}

async function hoverCard(page: Page, item: Video) {
  await card(page, item)
    .locator("[data-preview-media]")
    .hover({
      position: { x: 100, y: 50 },
    });
}

function previewRequests(requests: Request[]) {
  return requests.filter((request) =>
    /\/api\/videos\/\d+\/preview(?:\?|$)/.test(request.url()),
  );
}

function forbiddenRequests(requests: Request[]) {
  return requests.filter((request) => {
    const url = request.url();
    return (
      /\/api\/videos\/\d+\/(stream|transcode\.mp4)(?:\?|$)/.test(url) ||
      (/\/api\/videos\/\d+\/progress(?:\?|$)/.test(url) &&
        ["POST", "PUT"].includes(request.method()))
    );
  });
}

function watchRequests(page: Page): Request[] {
  const requests: Request[] = [];
  page.on("request", (request) => requests.push(request));
  return requests;
}

async function stablePageAriaSnapshot(page: Page) {
  let previous: string | undefined;
  let consecutiveMatches = 0;
  const deadline = Date.now() + 5_000;
  while (Date.now() < deadline) {
    const current = await page.ariaSnapshot();
    consecutiveMatches = current === previous ? consecutiveMatches + 1 : 0;
    previous = current;
    if (consecutiveMatches >= 4) return current;
    await page.waitForTimeout(200);
  }
  throw new Error("page accessibility tree did not stabilize");
}

async function liveRegionSnapshot(page: Page) {
  return page.locator("[aria-live]").evaluateAll((elements) =>
    elements.map((element) => ({
      live: element.getAttribute("aria-live"),
      role: element.getAttribute("role"),
      text: element.textContent?.trim() ?? "",
    })),
  );
}

async function tabTo(page: Page, target: ReturnType<Page["locator"]>) {
  for (let attempt = 0; attempt < 40; attempt += 1) {
    await page.keyboard.press("Tab");
    if (await target.evaluate((element) => element === document.activeElement)) return;
  }
  throw new Error("target was not reachable with Tab");
}

async function snapshot(directory: string): Promise<Map<string, string>> {
  const result = new Map<string, string>();
  for (const name of (await readdir(directory)).sort()) {
    const content = await readFile(path.join(directory, name));
    result.set(name, createHash("sha256").update(content).digest("hex"));
  }
  return result;
}

async function waitForScan(request: APIRequestContext) {
  await expect
    .poll(
      async () => {
        const response = await request.get("/api/scans/current");
        if (!response.ok()) return "missing";
        return ((await response.json()) as { state: string }).state;
      },
      { timeout: 60_000 },
    )
    .toBe("done");
}

async function waitForPreviews(request: APIRequestContext) {
  const required = ["direct", "container-only", "video-only"];
  await expect
    .poll(
      async () => {
        const response = await request.get("/api/videos?limit=100");
        expect(response.ok()).toBe(true);
        const result = (await response.json()) as VideoPage;
        for (const item of result.items) videos.set(item.title, item);
        return required.filter((title) => {
          const item = videos.get(title);
          return item?.previewState === "done" && item.previewUrl !== undefined;
        });
      },
      { timeout: 180_000 },
    )
    .toEqual(required);
}

async function hoverAndWaitForPlayback(page: Page, item: Video) {
  await hoverCard(page, item);
  const preview = card(page, item).locator("video");
  await expect(preview).toHaveCount(1, { timeout: 5_000 });
  await expect
    .poll(() => preview.evaluate((element) => element.currentTime), { timeout: 10_000 })
    .toBeGreaterThan(0.1);
  return preview;
}

async function seekPreviewFrame(preview: ReturnType<Page["locator"]>, seconds: number) {
  await preview.evaluate(
    (element, target) =>
      new Promise<void>((resolve) => {
        element.addEventListener("seeked", () => resolve(), { once: true });
        element.currentTime = target;
      }),
    seconds,
  );
}

async function assertNoPreviewAfterPointer(
  page: Page,
  item: Video,
  pointerType: "touch" | "pen",
) {
  const target = card(page, item);
  // React synthesizes onPointerEnter from bubbling pointerover events.
  await target.dispatchEvent("pointerover", {
    pointerType,
    bubbles: true,
  });
  await elapse(page, 500);
  await expect(target.locator("video")).toHaveCount(0);
}

async function screenshot(page: Page, width: number, state: string) {
  if (screenshotDir === undefined) return;
  await mkdir(screenshotDir, { recursive: true });
  await page.screenshot({
    path: path.join(
      screenshotDir,
      `20260923-hover-preview-${String(width)}-${state}.png`,
    ),
    fullPage: true,
  });
}

test.describe.serial("library hover preview", () => {
  test.beforeAll(async ({ request }) => {
    test.setTimeout(240_000);
    const mediaDir = process.env.MDM_E2E_MEDIA_DIR;
    if (mediaDir === undefined) throw new Error("MDM_E2E_MEDIA_DIR is not configured");
    sourceSnapshot = await snapshot(mediaDir);

    const created = await request.post("/api/media-folders", {
      headers: mutationHeaders,
      data: { path: mediaDir },
    });
    expect(created.status()).toBe(201);
    folder = (await created.json()) as MediaFolder;

    const scan = await request.post("/api/scans", {
      headers: mutationHeaders,
      data: {},
    });
    expect(scan.status()).toBe(202);
    await waitForScan(request);
    await waitForPreviews(request);
  });

  test.afterAll(async ({ request }) => {
    const mediaDir = process.env.MDM_E2E_MEDIA_DIR;
    if (mediaDir === undefined) throw new Error("MDM_E2E_MEDIA_DIR is not configured");
    expect(await snapshot(mediaDir)).toEqual(sourceSnapshot);
    if (folder !== undefined) {
      const removed = await request.delete(
        `/api/media-folders/${String(folder.id)}?version=${String(folder.version)}`,
        { headers: mutationHeaders },
      );
      expect(removed.status()).toBe(204);
    }
  });

  test("400ms後にpreviewだけを取得し、別cardへ移ると前のvideoを解放する", async ({
    page,
  }) => {
    test.setTimeout(30_000);
    const first = video("video-only");
    const second = video("container-only");
    expect(first.playable).toBe(false);
    expect(first.previewState).toBe("done");
    expect(first.previewUrl).toBeDefined();

    const requests = watchRequests(page);
    await page.goto("/");
    await expect(card(page, first)).toBeVisible();
    await expect(card(page, first).locator("video")).toHaveCount(0);
    await page.evaluate(() => {
      const state = { max: 0 };
      Object.assign(window, { __maxHoverPreviewVideos: state });
      new MutationObserver(() => {
        state.max = Math.max(
          state.max,
          document.querySelectorAll("article video").length,
        );
      }).observe(document.body, { childList: true, subtree: true });
    });

    await hoverCard(page, first);
    await page.waitForTimeout(300);
    expect(previewRequests(requests)).toHaveLength(0);
    expect(forbiddenRequests(requests)).toHaveLength(0);

    const firstPreview = card(page, first).locator("video");
    await expect(firstPreview).toHaveCount(1, { timeout: 5_000 });
    const firstTime = await firstPreview.evaluate((element) => element.currentTime);
    await expect
      .poll(() => firstPreview.evaluate((element) => element.currentTime))
      .toBeGreaterThan(firstTime + 0.1);
    await expect(card(page, first).getByText("読み取れませんでした")).toHaveCount(0);

    await hoverCard(page, second);
    await expect(card(page, first).locator("video")).toHaveCount(0);
    await expect(card(page, first).locator("img")).toBeVisible();
    await expect(card(page, second).locator("video")).toHaveCount(1, {
      timeout: 5_000,
    });
    await expect(page.locator("article video")).toHaveCount(1);
    expect(
      await page.evaluate(
        () =>
          (
            window as typeof window & {
              __maxHoverPreviewVideos: { max: number };
            }
          ).__maxHoverPreviewVideos.max,
      ),
    ).toBeLessThanOrEqual(1);

    expect(previewRequests(requests)).toHaveLength(2);
    expect(
      previewRequests(requests).every((request) => request.url().includes("/preview?v=")),
    ).toBe(true);
    expect(forbiddenRequests(requests)).toHaveLength(0);
  });

  test("pending、failed、previewUrl欠落とfocus/touch/penはpreviewを作らない", async ({
    page,
  }) => {
    const ineligibleTitles = ["direct-fallback", "audio-only", "silent"];
    const variants: Video[] = ineligibleTitles.map((title, index) => {
      const item = video(title);
      const previewState = index === 0 ? "pending" : index === 1 ? "failed" : "done";
      const copy = { ...item, previewState } as Video;
      delete copy.previewUrl;
      return copy;
    });

    await page.route("**/api/videos?*", async (route) => {
      const response = await route.fetch();
      const body = (await response.json()) as VideoPage;
      const byID = new Map(variants.map((item) => [item.id, item]));
      await route.fulfill({
        response,
        json: {
          ...body,
          items: body.items.map((item) => byID.get(item.id) ?? item),
        },
      });
    });
    // 一覧は準備中の項目を1件ずつ取り直すので、1件の応答も同じ状態にそろえる。
    await page.route(/\/api\/videos\/\d+$/, async (route) => {
      const id = Number(new URL(route.request().url()).pathname.split("/").at(-1));
      const variant = variants.find((item) => item.id === id);
      if (variant === undefined) {
        await route.fallback();
        return;
      }
      const response = await route.fetch();
      const body = (await response.json()) as Video;
      await route.fulfill({ response, json: { ...body, ...variant } });
    });

    const requests = watchRequests(page);
    await page.clock.install();
    await page.goto("/");
    for (const item of variants) {
      await hoverCard(page, item);
      await elapse(page, 500);
      await expect(card(page, item).locator("video")).toHaveCount(0);
    }

    const eligible = video("direct");
    const link = card(page, eligible).getByRole("link", {
      name: eligible.title,
      exact: true,
    });
    await link.focus();
    await elapse(page, 500);
    await expect(card(page, eligible).locator("video")).toHaveCount(0);
    await assertNoPreviewAfterPointer(page, eligible, "touch");
    await assertNoPreviewAfterPointer(page, eligible, "pen");
    await card(page, eligible).locator("[data-preview-checkbox]").hover();
    await elapse(page, 500);
    await expect(card(page, eligible).locator("video")).toHaveCount(0);

    expect(previewRequests(requests)).toHaveLength(0);
    expect(forbiddenRequests(requests)).toHaveLength(0);
  });

  test("touch主体contextでもmouse hoverは再生し、accessibility treeを変えない", async ({
    browser,
  }) => {
    const item = video("direct");
    const context = await browser.newContext({
      hasTouch: true,
      viewport: { width: 768, height: 800 },
    });
    const page = await context.newPage();
    const requests = watchRequests(page);
    try {
      await page.goto("/");
      const target = card(page, item);
      await expect(target).toBeVisible();
      const before = await stablePageAriaSnapshot(page);
      const liveRegions = await liveRegionSnapshot(page);
      await hoverAndWaitForPlayback(page, item);
      expect(await page.ariaSnapshot()).toBe(before);
      expect(await liveRegionSnapshot(page)).toEqual(liveRegions);
      await expect(
        target.getByRole("link", { name: item.title, exact: true }),
      ).toHaveCount(1);
      await expect(target.locator("[aria-live]")).toHaveCount(0);
      await expect(target.locator("video[controls]")).toHaveCount(0);
      await expect(target.locator("video")).toHaveAttribute("aria-hidden", "true");
      await page.mouse.move(0, 0);
      await expect(target.locator("video")).toHaveCount(0);
      await expect(target.locator("img")).toBeVisible();
      expect(await page.ariaSnapshot()).toBe(before);
      expect(await liveRegionSnapshot(page)).toEqual(liveRegions);
      expect(previewRequests(requests)).toHaveLength(1);
      expect(forbiddenRequests(requests)).toHaveLength(0);
    } finally {
      await context.close();
    }
  });

  test("keyboard操作はfocus、選択、Esc解除、Enter遷移だけを行いpreviewを作らない", async ({
    page,
  }) => {
    const item = video("direct");
    const requests = watchRequests(page);

    await page.clock.install();
    await page.goto("/");
    const target = card(page, item);
    const link = target.getByRole("link", { name: item.title, exact: true });
    const checkbox = target.getByRole("checkbox", { name: `「${item.title}」を選択` });

    await tabTo(page, checkbox);
    await expect(checkbox).toBeFocused();
    await page.keyboard.press("Space");
    await expect(checkbox).toBeChecked();
    await expect(page.getByText("1 件を選択中")).toBeVisible();
    await page.keyboard.press("Escape");
    await expect(checkbox).not.toBeChecked();
    await expect(page.getByText("1 件を選択中")).toHaveCount(0);

    await page.keyboard.press("Tab");
    await expect(link).toBeFocused();
    await elapse(page, 500);

    await expect(target.locator("video")).toHaveCount(0);
    await expect(target.locator("video[controls]")).toHaveCount(0);
    expect(previewRequests(requests)).toHaveLength(0);
    expect(forbiddenRequests(requests)).toHaveLength(0);

    await page.keyboard.press("Enter");
    await expect(page).toHaveURL(new RegExp(`/videos/${String(item.id)}$`));
    expect(previewRequests(requests)).toHaveLength(0);
  });

  test("reduced motionでもpreviewが進み、装飾transition durationは実質ゼロ", async ({
    page,
  }) => {
    const item = video("direct");
    await page.emulateMedia({ reducedMotion: "reduce" });
    await page.goto("/");
    const preview = await hoverAndWaitForPlayback(page, item);
    const durations = await card(page, item).evaluate((element) => {
      const media = element.querySelector<HTMLElement>("[data-preview-media]");
      return [
        getComputedStyle(element).transitionDuration,
        media === null ? "missing" : getComputedStyle(media).transitionDuration,
      ];
    });
    expect(durations).not.toContain("missing");
    for (const duration of durations.flatMap((value) => value.split(","))) {
      const seconds = duration.trim().endsWith("ms")
        ? Number.parseFloat(duration) / 1000
        : Number.parseFloat(duration);
      expect(seconds).toBeLessThanOrEqual(0.001);
    }
    await expect
      .poll(() => preview.evaluate((element) => element.currentTime))
      .toBeGreaterThan(0.1);
  });

  test("360/768/1280のbefore、playing、error fallbackを保存する", async ({ browser }) => {
    test.skip(screenshotDir === undefined, "MDM_E2E_SCREENSHOT_DIR is not set");
    test.setTimeout(90_000);
    const item = video("video-only");

    for (const width of [360, 768, 1280]) {
      const context = await browser.newContext({ viewport: { width, height: 800 } });
      const page = await context.newPage();
      try {
        await page.route("**/api/videos?*", async (route) => {
          const response = await route.fetch();
          const body = (await response.json()) as VideoPage;
          await route.fulfill({
            response,
            json: {
              ...body,
              items: body.items.map((candidate) =>
                candidate.id === item.id
                  ? {
                      ...candidate,
                      progress:
                        width === 768
                          ? { positionMs: 0, completed: true, updatedAt: "" }
                          : { positionMs: 3_000, completed: false, updatedAt: "" },
                    }
                  : candidate,
              ),
            },
          });
        });
        await page.goto("/");
        await expect(card(page, item)).toBeVisible();
        await screenshot(page, width, "before");

        const preview = await hoverAndWaitForPlayback(page, item);
        await seekPreviewFrame(preview, 4);
        await screenshot(page, width, "playing");

        await page.mouse.move(0, 0);
        await expect(card(page, item).locator("video")).toHaveCount(0);
        await page.route(`**/api/videos/${String(item.id)}/preview?*`, (route) =>
          route.abort("failed"),
        );
        const failedRequest = page.waitForRequest((request) =>
          request.url().includes(`/api/videos/${String(item.id)}/preview?`),
        );
        await hoverCard(page, item);
        await failedRequest;
        await expect(card(page, item).locator("video")).toHaveCount(0, {
          timeout: 5_000,
        });
        await expect(card(page, item).locator("img")).toBeVisible();
        await screenshot(page, width, "error-fallback");
      } finally {
        await context.close();
      }
    }
  });
});
