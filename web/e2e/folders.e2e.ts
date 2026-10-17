import { mkdir } from "node:fs/promises";
import path from "node:path";

import { expect, test, type APIRequestContext, type Page } from "@playwright/test";

// フォルダ画面（親 Issue #145・#195）のうち、本物のサーバーとブラウザでしか
// 確かめられないものだけを流す（docs/design-docs/web-testing.md）。長い名前の三幅での
// 横のはみ出しと、取り込んだフォルダをキーボードでたどるときのフォーカスの見え方である。
// 並び順・パンくず・空の案内・検索・絞り込み・取り込みでフォルダが無くなったときの案内の
// 振る舞いは Vitest（src/folders/FolderPage、
// src/folders/folderPath）が、子フォルダの自然順・直下だけの一覧・配下の検索は
// Go のテスト（internal/domain/folder_test.go、internal/httpapi/folders_test.go、
// internal/store/folders_test.go）が確かめる。
// 構成は run-e2e.mjs が generateFolderFixtures で作る。

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
  // 取り込めなかったファイルがあれば、ここで結果を見てすぐに落とす。
  const scan = await currentScan();
  expect({ state: scan.state, failed: scan.issues.failed }).toEqual({
    state: "done",
    failed: 0,
  });
}

/** tabUntil は条件を満たす要素にフォーカスが来るまで Tab を押す。 */
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
        // 輪郭は要素自身か、カードなら外側の箱（overflow-hidden で切れるため）に出る。
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
