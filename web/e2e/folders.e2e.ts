import { mkdir } from "node:fs/promises";
import path from "node:path";

import { expect, test, type APIRequestContext, type Page } from "@playwright/test";

// Verify long-name overflow and keyboard focus with real layout; lower-level suites cover folder behavior. Fixtures come from generateFolderFixtures; see docs/design-docs/web-testing.md.

interface MediaFolder {
  id: number;
  version: number;
  path: string;
}

const origin = "http://127.0.0.1:15173";
const mutationHeaders = { Origin: origin, "Content-Type": "application/json" };
const registered: MediaFolder[] = [];

function rootA(): number {
  const found = registered.find((folder) =>
    path.normalize(folder.path).endsWith(path.join("a", "movies")),
  );
  if (found === undefined) throw new Error("a/movies is not registered");
  return found.id;
}

function folderUrl(rootId: number, path = ""): string {
  const segments = path === "" ? [] : path.split("/").map(encodeURIComponent);
  return ["/folders", String(rootId), ...segments].join("/");
}

async function waitForLibrary(request: APIRequestContext, scanId: number) {
  const currentScan = async () => {
    const response = await request.get("/api/scans/current");
    return (await response.json()) as {
      id: number;
      state: string;
      issues: { failed: number };
    };
  };
  await expect
    .poll(
      async () => {
        const scan = await currentScan();
        return scan.id === scanId && scan.state !== "running" ? "finished" : scan.state;
      },
      { timeout: 60_000 },
    )
    .toBe("finished");
  // Fail immediately if any fixture failed to import.
  const scan = await currentScan();
  expect({ state: scan.state, failed: scan.issues.failed }).toEqual({
    state: "done",
    failed: 0,
  });
}

async function tabUntil(
  page: Page,
  matches: (element: {
    tag: string;
    label: string;
    href: string;
    text: string;
  }) => boolean,
  key: "Tab" | "Shift+Tab" = "Tab",
) {
  for (let step = 0; step < 60; step += 1) {
    await page.keyboard.press(key);
    const active = await page.evaluate(() => {
      const element = document.activeElement;
      return {
        tag: element?.tagName ?? "",
        label: element?.getAttribute("aria-label") ?? "",
        href: element?.getAttribute("href") ?? "",
        text: element?.textContent?.trim() ?? "",
        // Cards draw focus on the outer wrapper to avoid clipping by overflow-hidden.
        visible:
          (element?.matches(":focus-visible") ?? false) &&
          [element, element?.closest("article")].some((box) => {
            if (!box) return false;
            const style = getComputedStyle(box);
            return style.outlineStyle !== "none" && parseFloat(style.outlineWidth) > 0;
          }),
      };
    });
    if (matches(active)) {
      expect(active.visible, "キーボードフォーカスが見えている").toBe(true);
      return;
    }
  }
  throw new Error("focus target was not reached");
}

test.describe.serial("folder browser", () => {
  test.beforeAll(async ({ request }) => {
    test.setTimeout(420_000);
    const root = process.env.MDM_E2E_FOLDERS_MEDIA_DIR;
    if (root === undefined)
      throw new Error("MDM_E2E_FOLDERS_MEDIA_DIR is not configured");

    for (const relative of ["a/movies", "b/movies"]) {
      const created = await request.post("/api/media-folders", {
        headers: mutationHeaders,
        data: { path: `${root}/${relative}` },
      });
      expect(created.status()).toBe(201);
      registered.push((await created.json()) as MediaFolder);
    }
    const scan = await request.post("/api/scans", { headers: mutationHeaders, data: {} });
    expect(scan.status()).toBe(202);
    await waitForLibrary(request, ((await scan.json()) as { id: number }).id);
  });

  test.afterAll(async ({ request }) => {
    const response = await request.get("/api/media-folders");
    const current = (await response.json()) as MediaFolder[];
    for (const folder of current.filter((item) =>
      registered.some((mine) => mine.id === item.id),
    )) {
      const removed = await request.delete(
        `/api/media-folders/${String(folder.id)}?version=${String(folder.version)}`,
        { headers: mutationHeaders },
      );
      expect(removed.status()).toBe(204);
    }
  });

  test("空の登録ルートと長い名前を三幅で確認できる", async ({ page, request }) => {
    const mediaRoot = process.env.MDM_E2E_FOLDERS_MEDIA_DIR;
    if (mediaRoot === undefined) throw new Error("folder fixtures are not configured");
    const name = "長い日本語のフォルダ名 and a long English folder name";
    const emptyPath = path.join(mediaRoot, name);
    await mkdir(emptyPath, { recursive: true });
    const created = await request.post("/api/media-folders", {
      headers: mutationHeaders,
      data: { path: emptyPath },
    });
    expect(created.status()).toBe(201);
    const empty = (await created.json()) as MediaFolder;
    try {
      for (const width of [360, 768, 1280]) {
        await page.setViewportSize({ width, height: 800 });
        await page.goto("/folders");
        const card = page.getByRole("link", { name: new RegExp(`^${name}, 0 videos`) });
        await expect(card).toBeVisible();
        await card.click();
        await expect(page).toHaveURL(folderUrl(empty.id));
        expect(
          await page.evaluate(
            () =>
              document.documentElement.scrollWidth <=
              document.documentElement.clientWidth,
          ),
        ).toBe(true);
      }
    } finally {
      const removed = await request.delete(
        `/api/media-folders/${String(empty.id)}?version=${String(empty.version)}`,
        { headers: mutationHeaders },
      );
      expect(removed.status()).toBe(204);
    }
  });

  test("キーボードだけでフォルダをたどって動画へ行き、パンくずで戻れる", async ({
    page,
  }) => {
    await page.setViewportSize({ width: 1280, height: 800 });
    await page.goto("/library");
    await tabUntil(page, (active) => active.href === "/folders");
    await page.keyboard.press("Enter");
    await expect(page).toHaveURL(/\/folders$/);

    await tabUntil(page, (active) => /^movies, .*[\\/]a[\\/]movies$/.test(active.label));
    await page.keyboard.press("Enter");
    await expect(page).toHaveURL(folderUrl(rootA()));

    await tabUntil(page, (active) => active.label.startsWith("A, "));
    await page.keyboard.press("Enter");
    await expect(page).toHaveURL(folderUrl(rootA(), "A"));

    await tabUntil(page, (active) => active.href.startsWith("/videos/"));
    await tabUntil(page, (active) => active.text === "movies", "Shift+Tab");
    await page.keyboard.press("Enter");
    await expect(page).toHaveURL(folderUrl(rootA()));
  });
});
