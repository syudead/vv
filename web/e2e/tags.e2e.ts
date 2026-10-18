import { expect, test, type APIRequestContext, type Page } from "@playwright/test";

// Verify tag layout in the browser; lower-level suites cover behavior. The fixture count positions the last grid row near the bottom at 1280 x 800; see docs/design-docs/web-testing.md.

const videoCount = 16;

interface MediaFolder {
  id: number;
  version: number;
}

interface Video {
  id: number;
  title: string;
}

interface Tag {
  id: number;
  name: string;
  synonyms: string[];
  videoCount: number;
}

const origin = "http://127.0.0.1:15173";
const mutationHeaders = { Origin: origin, "Content-Type": "application/json" };
let folder: MediaFolder | undefined;
const videos = new Map<string, Video>();

function video(title: string): Video {
  const found = videos.get(title);
  if (found === undefined) throw new Error(`video not indexed: ${title}`);
  return found;
}

async function waitForScan(request: APIRequestContext, scanId: number) {
  await expect
    .poll(
      async () => {
        const response = await request.get("/api/scans/current");
        const scan = (await response.json()) as { id: number; state: string };
        return scan.id === scanId && scan.state !== "running" ? scan.state : "running";
      },
      { timeout: 60_000 },
    )
    .toBe("done");
}

async function createTag(request: APIRequestContext, name: string): Promise<Tag> {
  const response = await request.post("/api/tags", {
    headers: mutationHeaders,
    data: { name },
  });
  expect(response.status()).toBe(201);
  return (await response.json()) as Tag;
}

async function attachTag(request: APIRequestContext, videoId: number, tagId: number) {
  const response = await request.post("/api/video-tags", {
    headers: mutationHeaders,
    data: { videoIds: [videoId], action: "add", tag: { id: tagId } },
  });
  expect(response.ok()).toBe(true);
}

/** Remove inherited tags so the target chip stays directly clickable instead of moving into overflow. */
async function clearVideoTags(request: APIRequestContext, videoId: number) {
  const response = await request.get(`/api/videos/${String(videoId)}`);
  const data = (await response.json()) as { tags: { id: number }[] };
  for (const tag of data.tags) {
    const removed = await request.post("/api/video-tags", {
      headers: mutationHeaders,
      data: { videoIds: [videoId], action: "remove", tag: { id: tag.id } },
    });
    expect(removed.ok()).toBe(true);
  }
}

test.describe.serial("video tags", () => {
  test.beforeAll(async ({ request }) => {
    test.setTimeout(120_000);
    const root = process.env.MDM_E2E_TAGS_MEDIA_DIR;
    if (root === undefined) throw new Error("MDM_E2E_TAGS_MEDIA_DIR is not configured");

    const created = await request.post("/api/media-folders", {
      headers: mutationHeaders,
      data: { path: root },
    });
    expect(created.status()).toBe(201);
    folder = (await created.json()) as MediaFolder;

    const scan = await request.post("/api/scans", { headers: mutationHeaders, data: {} });
    expect(scan.status()).toBe(202);
    const scanId = ((await scan.json()) as { id: number }).id;
    await waitForScan(request, scanId);

    const list = await request.get("/api/videos?limit=200");
    const page = (await list.json()) as { items: Video[] };
    expect(page.items).toHaveLength(videoCount);
    for (const item of page.items) videos.set(item.title, item);
  });

  test.afterAll(async ({ request }) => {
    if (folder === undefined) return;
    const removed = await request.delete(
      `/api/media-folders/${String(folder.id)}?version=${String(folder.version)}`,
      { headers: mutationHeaders },
    );
    expect(removed.status()).toBe(204);
  });

  test.describe("タグ管理画面（issue 271）", () => {
    /** Find rows through the tag link accessible name. */
    function tagRowByName(page: Page, name: string) {
      // Use exact matching to distinguish case-sensitive tag names such as Anime and anime.
      return page
        .getByRole("link", { name: `Open the library filtered by ${name}`, exact: true })
        .locator("xpath=ancestor::*[@data-tag-id][1]");
    }

    /** Below sm (640 px), Actions replaces the CSS-hidden More actions menu. */
    function rowMenuName(width: number) {
      return width < 640 ? "Actions" : "More actions";
    }

    /** Scroll until the virtualized row mounts, restarting from the top if needed. Use a deadline so intermediate layout resets cannot be mistaken for the end. */
    async function revealTagRow(page: Page, name: string) {
      const row = tagRowByName(page, name);
      await expect(
        page.getByRole("link", { name: /^Open the library filtered by / }).first(),
      ).toBeVisible();
      await page.evaluate(() => window.scrollTo(0, 0));
      // Include scrolling into view in each attempt because remeasurement may unmount the row.
      await expect(async () => {
        if ((await row.count()) === 0) {
          await page.evaluate(() => {
            const before = window.scrollY;
            window.scrollBy(0, 400);
            if (window.scrollY === before) window.scrollTo(0, 0);
          });
          await expect(row).toBeAttached({ timeout: 300 });
        }
        await row.scrollIntoViewIfNeeded({ timeout: 1_000 });
        await expect(row).toBeVisible({ timeout: 300 });
      }).toPass({ timeout: 15_000, intervals: [0] });
      return row;
    }

    test("16b: 仮のタグの行も名前の列の空白を押すと絞り込んだ一覧が開き、長い名前でも目印と操作が行に残る", async ({
      page,
      request,
    }) => {
      // Tentative tags can only be created through the external API.
      const issued = await request.post("/api/api-tokens", {
        headers: mutationHeaders,
        data: { name: "e2e仮のタグ" },
      });
      expect(issued.status()).toBe(201);
      const { secret } = (await issued.json()) as { secret: string };
      const shortName = "e2e仮移動先";
      const longName = `e2e仮長い名前${"あ".repeat(80)}`;
      const a = video("タグ動画A");
      await clearVideoTags(request, a.id);
      const tagged = await request.post("/api/v1/video-tags", {
        headers: {
          Authorization: `Bearer ${secret}`,
          "Content-Type": "application/json",
        },
        data: {
          videos: [{ id: a.id }],
          action: "add",
          tags: [shortName, longName],
          tentative: true,
        },
      });
      expect(tagged.ok()).toBe(true);

      for (const width of [1280, 360]) {
        await page.setViewportSize({ width, height: 800 });
        await page.goto("/tags");
        for (const name of [shortName, longName]) {
          await revealTagRow(page, name);
          const row = tagRowByName(page, name);
          const link = row.getByRole("link", {
            name: `Open the library filtered by ${name}`,
            exact: true,
          });
          const mark = row.locator("svg.lucide-circle-dashed");
          await expect(mark).toBeVisible();
          // The marker stays after the name on the same line; spacing is not part of this assertion.
          const linkBox = await link.boundingBox();
          const markBox = await mark.boundingBox();
          if (linkBox === null || markBox === null) throw new Error("no layout");
          expect(markBox.x).toBeGreaterThanOrEqual(linkBox.x + linkBox.width);
          expect(markBox.y).toBeLessThan(linkBox.y + linkBox.height);
          expect(markBox.y + markBox.height).toBeGreaterThan(linkBox.y);

          const rowBox = await row.boundingBox();
          const moreBox = await row
            .getByRole("button", { name: rowMenuName(width) })
            .boundingBox();
          if (rowBox === null || moreBox === null) throw new Error("no layout");
          expect(moreBox.x + moreBox.width).toBeLessThanOrEqual(rowBox.x + rowBox.width);
        }

        await revealTagRow(page, longName);
        const longLink = tagRowByName(page, longName).getByRole("link", {
          name: `Open the library filtered by ${longName}`,
          exact: true,
        });
        expect(
          await longLink.evaluate((node) => node.scrollWidth > node.clientWidth),
        ).toBe(true);

        expect(
          await page.evaluate(
            () => document.documentElement.scrollWidth <= window.innerWidth,
          ),
        ).toBe(true);
      }
      await page.setViewportSize({ width: 1280, height: 800 });

      // Click the blank area after the short name and marker.
      await revealTagRow(page, shortName);
      const row = tagRowByName(page, shortName);
      const link = row.getByRole("link", {
        name: `Open the library filtered by ${shortName}`,
        exact: true,
      });
      const href = await link.getAttribute("href");
      const column = link.locator("xpath=..");
      const columnBox = await column.boundingBox();
      const markBox = await row.locator("svg.lucide-circle-dashed").boundingBox();
      if (columnBox === null || markBox === null) throw new Error("no layout");
      const x = columnBox.width - 4;
      expect(columnBox.x + x).toBeGreaterThan(markBox.x + markBox.width);
      await column.click({ position: { x, y: columnBox.height / 2 } });
      await expect(page).toHaveURL(`${origin}${String(href)}`);
    });

    test("狭い幅でも見出しの件数は折り返さず、「Rejected names」のタブを省略せずに本文の中に収める", async ({
      page,
    }) => {
      // Four-digit counts maximize heading and tab widths.
      const createdAt = "2026-01-01T00:00:00Z";
      const items = Array.from({ length: 1000 }, (_, index) => ({
        id: 900000 + index,
        name: `e2e狭い幅${String(index).padStart(4, "0")}`,
        synonyms: [],
        videoCount: 0,
        tentative: true,
        // kind is required; null means unclassified, while omission breaks the badge.
        kind: null,
        createdAt,
      }));
      // Paginated requests include sort, limit and tentative parameters.
      await page.route(/\/api\/tags(\?.*)?$/, (route) =>
        route.request().method() === "GET"
          ? route.fulfill({
              json: { items, total: items.length, totalAll: items.length },
            })
          : route.fallback(),
      );
      await page.route("**/api/tags/rejected-names**", (route) =>
        route.request().method() === "GET"
          ? route.fulfill({
              json: {
                items: Array.from(
                  { length: 1000 },
                  (_, index) => `e2e却下${String(index)}`,
                ),
                total: 1000,
              },
            })
          : route.fallback(),
      );
      for (const width of [320, 360]) {
        await page.setViewportSize({ width, height: 800 });

        await page.goto("/tags?tentative=1");
        const count = page.getByRole("status").filter({ hasText: "1,000 of 1,000 tags" });
        await expect(count).toBeVisible();
        const tab = page.getByRole("tab", { name: /^Rejected names/ });
        await expect(tab).toBeVisible();
        await expect(tab).toContainText("1,000");
        const tabBox = await tab.boundingBox();
        if (tabBox === null) throw new Error("no layout");
        // A wrapped count produces multiple client rectangles.
        expect(
          await count.evaluate((element) => {
            const range = document.createRange();
            range.selectNodeContents(element);
            return new Set(
              [...range.getClientRects()].map((rect) => Math.round(rect.top)),
            ).size;
          }),
        ).toBe(1);

        expect(tabBox.x + tabBox.width).toBeLessThanOrEqual(width);

        expect(
          await page.evaluate(
            () => document.documentElement.scrollWidth <= window.innerWidth,
          ),
        ).toBe(true);
        // The third tab must remain reachable by scrolling when it does not fit.
        const kinds = page.getByRole("tab", { name: /^Kinds/ });
        await kinds.click();
        await expect(page).toHaveURL(/[?&]tab=kinds/);
        // The scroll strip must not clip the focus ring extending 4 px outside the tab.
        await kinds.press("ArrowLeft");
        await expect(tab).toBeFocused();
        expect(
          await tab.evaluate((element) => {
            const strip = element.closest('[role="tablist"]')?.parentElement;
            if (strip === null || strip === undefined) return false;
            const outer = strip.getBoundingClientRect();
            const inner = element.getBoundingClientRect();
            return outer.top <= inner.top - 4 && outer.bottom >= inner.bottom + 4;
          }),
        ).toBe(true);
        // The selected tab stays visible after direct navigation and back navigation.
        const kindsInView = async () => {
          await expect(kinds).toHaveAttribute("aria-selected", "true");
          await expect
            .poll(async () => {
              const box = await kinds.boundingBox();
              return box !== null && box.x >= 0 && box.x + box.width <= width;
            })
            .toBe(true);
        };
        await page.goto("/tags?tentative=1&tab=kinds");
        await expect(kinds).toContainText(/\d/);
        await kindsInView();
        await page.getByRole("tab", { name: /^Tags/ }).click();
        await expect(page).not.toHaveURL(/[?&]tab=kinds/);
        // 押したタブへのフォーカスで帯が送られるのを描き終えてから戻る。人は押した同じ
        // コマのうちに戻らない。
        await page.evaluate(
          () =>
            new Promise((resolve) =>
              requestAnimationFrame(() => requestAnimationFrame(resolve)),
            ),
        );
        await page.goBack();
        await expect(page).toHaveURL(/[?&]tab=kinds/);
        await kindsInView();
      }
      await page.setViewportSize({ width: 1280, height: 800 });
    });

    test("B3: 1280幅で統合先の候補を開くと、8行すべてが見える", async ({
      page,
      request,
    }) => {
      await page.setViewportSize({ width: 1280, height: 800 });
      await createTag(request, "e2eB3統合元");
      const names: string[] = [];
      for (let i = 1; i <= 8; i += 1) {
        const name = `e2eB3統合先${String(i)}`;
        names.push(name);
        await createTag(request, name);
      }

      await page.goto("/tags");
      await revealTagRow(page, "e2eB3統合元");
      const row = tagRowByName(page, "e2eB3統合元");
      await row.getByRole("button", { name: "More actions" }).click();
      await page.getByRole("menuitem", { name: "Merge into another tag…" }).click();

      const dialog = page.getByRole("dialog", { name: 'Merge "e2eB3統合元"' });
      const combo = dialog.getByRole("combobox", { name: "Tag to merge into" });
      await combo.fill("e2eB3統合先");

      // All eight candidates must remain visible despite the dialog overflow container.
      for (const name of names) {
        await expect(page.getByRole("option", { name: new RegExp(name) })).toBeVisible();
      }

      // Click the last candidate to detect pointer-event interception too.
      await page.getByRole("option", { name: new RegExp(names[7]!) }).click();
      await expect(
        dialog.getByText(new RegExp(`get the tag "${names[7]!}"`)),
      ).toBeVisible();
    });
  });

  // Overflow chips wrap without truncation, stay inside the viewport and scroll internally when needed; see docs/design-docs/library-ui.md.
  test.describe("カードの「+N」の一覧（041-tag-overflow-list）", () => {
    /** A 60-character unbroken name forces wrapping within the chip. */
    const longName = (
      "e2e一覧の長い名前_" + "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
    ).slice(0, 60);

    /** Respect Radix collisionPadding of 8 px. */
    function expectInsideViewport(
      box: { x: number; y: number; width: number; height: number },
      viewport: { width: number; height: number },
    ) {
      const padding = 8 - 0.5;
      expect(box.x).toBeGreaterThanOrEqual(padding);
      expect(box.y).toBeGreaterThanOrEqual(padding);
      expect(box.x + box.width).toBeLessThanOrEqual(viewport.width - padding);
      expect(box.y + box.height).toBeLessThanOrEqual(viewport.height - padding);
    }

    test("1・5: タグ30個の格子の最後のカードで、一覧が画面内に開き、最後のタグまで届き、長い名前を全文で出す", async ({
      page,
      request,
    }) => {
      test.setTimeout(120_000);
      await page.setViewportSize({ width: 1280, height: 800 });
      await page.goto("/library");
      await expect(page.getByRole("article")).toHaveCount(videoCount);

      const lastId = await page.getByRole("article").last().getAttribute("data-video-id");
      if (lastId === null) throw new Error("no data-video-id on the last card");
      const targetId = Number(lastId);
      await clearVideoTags(request, targetId);
      const tagIds = new Map<string, number>();
      for (let index = 1; index < 30; index += 1) {
        const name = `e2e一覧${String(index).padStart(2, "0")}`;
        const tag = await createTag(request, name);
        tagIds.set(name, tag.id);
        await attachTag(request, targetId, tag.id);
      }
      const long = await createTag(request, longName);
      tagIds.set(longName, long.id);
      await attachTag(request, targetId, long.id);

      // Thirty chips fit at normal heights; 390 x 300 requires internal scrolling to reach the last chip.
      for (const { scrolls, ...viewport } of [
        { width: 1280, height: 800, scrolls: false },
        { width: 390, height: 844, scrolls: false },
        { width: 390, height: 300, scrolls: true },
      ]) {
        await page.setViewportSize(viewport);
        await page.goto("/library");
        await expect(page.getByRole("article")).toHaveCount(videoCount);
        const card = page.locator(`article[data-video-id="${String(targetId)}"]`);
        // Scroll beyond the last card to place the final grid row at the viewport bottom.
        await card.scrollIntoViewIfNeeded();
        await page.mouse.move(viewport.width / 2, viewport.height / 2);
        await page.mouse.wheel(0, 10_000);
        const overflow = card.getByRole("button", { name: /^Show \d+ more tags?$/ });
        await expect(overflow).toBeVisible();
        const moreBox = await overflow.boundingBox();
        if (moreBox === null) throw new Error("no layout for +N");
        if (viewport.width === 1280) {
          // There is no room for thirty chips below this trigger.
          expect(moreBox.y).toBeGreaterThan(viewport.height / 2);
        }

        await overflow.click();
        const dialog = page.getByRole("dialog");
        await expect(dialog).toBeVisible();
        // Wait for the opening scale animation before measuring.
        await dialog.evaluate((element) =>
          Promise.all(element.getAnimations().map((animation) => animation.finished)),
        );
        // Height must respect the remaining space reported by Radix.
        expect(
          await dialog.evaluate((element) => getComputedStyle(element).maxHeight),
        ).toMatch(/px$/);
        const dialogBox = await dialog.boundingBox();
        if (dialogBox === null) throw new Error("no layout for the list");
        expectInsideViewport(dialogBox, viewport);
        if (viewport.width === 1280) {
          // Near the viewport bottom, open above or beside the trigger.
          const above = dialogBox.y + dialogBox.height <= moreBox.y;
          const beside =
            dialogBox.x >= moreBox.x + moreBox.width ||
            dialogBox.x + dialogBox.width <= moreBox.x;
          expect(above || beside).toBe(true);
        }

        const longChip = dialog.getByRole("button", {
          name: `Filter by ${longName}`,
          exact: true,
        });
        await longChip.scrollIntoViewIfNeeded();
        const rendered = await longChip.evaluate((element) => {
          const name = element.firstElementChild as HTMLElement;
          const style = getComputedStyle(name);
          return {
            text: name.textContent,
            textOverflow: style.textOverflow,
            clipped: name.scrollWidth > name.clientWidth,
            lines: name.getBoundingClientRect().height / parseFloat(style.lineHeight),
          };
        });
        expect(rendered.text).toBe(longName);
        expect(rendered.textOverflow).not.toBe("ellipsis");
        expect(rendered.clipped).toBe(false);
        expect(rendered.lines).toBeGreaterThanOrEqual(2);

        const chips = dialog.getByRole("button");
        await expect(chips).toHaveCount(30 - (await visibleChipCount(card)));
        const lastChip = chips.last();
        const list = dialog.getByRole("list");
        if (scrolls) {
          // The last chip starts outside the visible scroll area.
          await list.evaluate((element) => element.scrollTo(0, 0));
          const overflow = await list.evaluate((element) => ({
            scrollHeight: element.scrollHeight,
            clientHeight: element.clientHeight,
          }));
          expect(overflow.scrollHeight).toBeGreaterThan(overflow.clientHeight);
          const listTop = await list.boundingBox();
          const hiddenBox = await lastChip.boundingBox();
          if (listTop === null || hiddenBox === null) throw new Error("no layout");
          expect(hiddenBox.y).toBeGreaterThanOrEqual(listTop.y + listTop.height);
        }
        await lastChip.scrollIntoViewIfNeeded();
        if (scrolls) {
          expect(await list.evaluate((element) => element.scrollTop)).toBeGreaterThan(0);
        }
        const lastBox = await lastChip.boundingBox();
        const listBox = await dialog.boundingBox();
        if (lastBox === null || listBox === null) throw new Error("no layout");
        expect(lastBox.y).toBeGreaterThanOrEqual(listBox.y);
        expect(lastBox.y + lastBox.height).toBeLessThanOrEqual(
          listBox.y + listBox.height,
        );
        await page.keyboard.press("Escape");
        await expect(page.getByRole("dialog")).toHaveCount(0);
      }
    });

    /** Count directly visible chips, excluding the overflow trigger. */
    async function visibleChipCount(card: ReturnType<Page["locator"]>) {
      return card
        .getByRole("list", { name: "Tags", exact: true })
        .getByRole("button", { name: /^Filter by / })
        .count();
    }
  });
});
