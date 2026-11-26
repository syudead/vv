import { expect, test, type APIRequestContext, type Page } from "@playwright/test";

// Verify the tag screens against the real server and browser layout; lower-level suites cover each rule. The fixture count positions the last grid row near the bottom at 1280 x 800; see docs/design-docs/web-testing.md.

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

/** Tags outlive a failed attempt, so each CI retry names its own tags instead of hitting 409 on the first one. */
function attemptName(base: string): string {
  return `${base}r${String(test.info().retry)}`;
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

  test.describe("タグカードとタグのページ（070-tag-workbench）", () => {
    async function addSynonym(request: APIRequestContext, tagId: number, name: string) {
      const response = await request.post(`/api/tags/${String(tagId)}/synonyms`, {
        headers: mutationHeaders,
        data: { name },
      });
      expect(response.ok()).toBe(true);
    }

    /** The library card of the video, found after the list has loaded. */
    function libraryCard(page: Page, videoId: number) {
      return page.locator(`article[data-video-id="${String(videoId)}"]`);
    }

    function chip(page: Page, videoId: number, name: string) {
      return libraryCard(page, videoId).getByRole("button", {
        name: `Filter by ${name}`,
        exact: true,
      });
    }

    async function openNameMenu(page: Page, name: string) {
      await page.getByRole("button", { name: `More actions for "${name}"` }).click();
    }

    test("受け入れ1: 代表名にしたシノニムをライブラリのカードが出し、旧名の検索でも同じ動画が見つかる", async ({
      page,
      request,
    }) => {
      const before = attemptName("e2e入替前の名前");
      const after = attemptName("e2e入替後の名前");
      const a = video("タグ動画A");
      await clearVideoTags(request, a.id);
      const tag = await createTag(request, before);
      await addSynonym(request, tag.id, after);
      await attachTag(request, a.id, tag.id);

      await page.goto(`/tags/${String(tag.id)}`);
      await expect(page.getByRole("heading", { level: 1, name: before })).toBeVisible();
      await page.getByRole("button", { name: `Make "${after}" the main name` }).click();
      await expect(page.getByRole("heading", { level: 1, name: after })).toBeVisible();
      await expect(page.locator(`[data-synonym="${before}"]`)).toBeVisible();

      await page.goto("/");
      await expect(chip(page, a.id, after)).toBeVisible();
      await expect(chip(page, a.id, before)).toHaveCount(0);

      await page.goto(`/?q=${encodeURIComponent(before)}`);
      await expect(libraryCard(page, a.id)).toBeVisible();
      await expect(page.getByRole("article")).toHaveCount(1);
    });

    test("受け入れ2: 別のタグへ移したシノニムで動画にタグを付けると、移動先のタグが付く", async ({
      page,
      request,
    }) => {
      const moved = attemptName("e2e移す名前");
      const source = await createTag(request, attemptName("e2e移動元"));
      const target = await createTag(request, attemptName("e2e移動先"));
      await addSynonym(request, source.id, moved);
      const b = video("タグ動画B");
      await clearVideoTags(request, b.id);

      await page.goto(`/tags/${String(source.id)}`);
      await openNameMenu(page, moved);
      await page.getByRole("menuitem", { name: "Move to another tag…" }).click();
      const dialog = page.getByRole("dialog", { name: `Move "${moved}" to another tag` });
      await dialog.getByRole("combobox", { name: "Tag to move it to" }).fill(target.name);
      // Clicking the candidate inside the dialog also catches an overlay that intercepts it.
      await page.getByRole("option", { name: new RegExp(target.name) }).click();
      await dialog.getByRole("button", { name: "Move", exact: true }).click();
      await expect(dialog).toHaveCount(0);
      await expect(page.locator(`[data-synonym="${moved}"]`)).toHaveCount(0);

      const tagged = await request.post("/api/video-tags", {
        headers: mutationHeaders,
        data: { videoIds: [b.id], action: "add", tag: { name: moved } },
      });
      expect(tagged.ok()).toBe(true);

      await page.goto(`/tags/${String(target.id)}`);
      await expect(page.locator(`[data-synonym="${moved}"]`)).toBeVisible();
      await page.goto("/");
      await expect(chip(page, b.id, target.name)).toBeVisible();
      await expect(chip(page, b.id, source.name)).toHaveCount(0);
    });

    test("受け入れ3: 独立させたシノニムが新しいタグとして一覧に出て、元のタグからは消える", async ({
      page,
      request,
    }) => {
      const detached = attemptName("e2e独立する名前");
      const source = await createTag(request, attemptName("e2e独立元"));
      await addSynonym(request, source.id, detached);

      await page.goto(`/tags/${String(source.id)}`);
      await openNameMenu(page, detached);
      await page.getByRole("menuitem", { name: "Make it a tag of its own" }).click();
      await expect(
        page.getByText(`"${detached}" is now a tag of its own.`),
      ).toBeVisible();
      await expect(page.locator(`[data-synonym="${detached}"]`)).toHaveCount(0);

      // The search also matches synonyms, so one card means the source no longer holds the name.
      await page.goto(`/tags?q=${encodeURIComponent(detached)}`);
      const cards = page.locator("a[data-tag-id]");
      await expect(cards).toHaveCount(1);
      await expect(cards).toContainText(detached);
      await expect(cards).not.toHaveAttribute("data-tag-id", String(source.id));
      await cards.click();
      await expect(page.getByRole("heading", { level: 1, name: detached })).toBeVisible();
    });

    test("360幅で一覧、タグのページ、種類と却下した名前のページが横にはみ出さず、40文字の名前と操作の名前を省略しない", async ({
      page,
      request,
    }) => {
      const longName = attemptName("e2e長い名前").padEnd(40, "あ");
      const synonym = attemptName("e2e長いシノニム").padEnd(40, "い");
      // No API makes a tentative tag to reject, so the list returns one long name.
      const rejectedName = "e2e却下した長い名前".padEnd(40, "う");
      await page.route("**/api/tags/rejected-names**", (route) =>
        route.request().method() === "GET"
          ? route.fulfill({ json: { items: [rejectedName], total: 1 } })
          : route.fallback(),
      );
      const tag = await createTag(request, longName);
      await addSynonym(request, tag.id, synonym);
      const width = 360;
      await page.setViewportSize({ width, height: 800 });
      const fitsWidth = () =>
        page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth);
      /** A visible label is the rendered text, so an action shrunk to its icon fails even though its aria-label stays. */
      async function expectInside(locator: ReturnType<Page["locator"]>, label?: string) {
        await expect(locator).toBeVisible();
        if (label !== undefined) {
          await expect(locator).toHaveText(label, { useInnerText: true });
          expect(
            await locator.evaluate((node) => node.scrollWidth <= node.clientWidth),
          ).toBe(true);
        }
        const box = await locator.boundingBox();
        if (box === null) throw new Error("no layout");
        expect(box.x).toBeGreaterThanOrEqual(0);
        expect(box.x + box.width).toBeLessThanOrEqual(width);
      }

      await page.goto("/tags");
      await expect(page.locator("a[data-tag-id]").first()).toBeVisible();
      for (const name of ["Kinds", "Rejected names", "New tag"]) {
        const action = page.getByRole(name === "New tag" ? "button" : "link", {
          name,
          exact: true,
        });
        await expectInside(action, name);
      }
      // The kind filter scrolls inside its own row under the actions instead of widening the page.
      const kindFilter = page.locator('[data-slot="page-header-filter"]');
      await expectInside(kindFilter);
      const actionsBox = await page
        .locator('[data-slot="page-header-actions"]')
        .boundingBox();
      const filterBox = await kindFilter.boundingBox();
      if (actionsBox === null || filterBox === null) throw new Error("no layout");
      expect(filterBox.y).toBeGreaterThanOrEqual(actionsBox.y + actionsBox.height);
      expect(await fitsWidth()).toBe(true);

      await page.goto(`/tags/${String(tag.id)}`);
      const title = page.getByRole("heading", { level: 1, name: longName });
      await expect(title).toBeVisible();
      await expectInside(title);
      expect(await title.evaluate((node) => node.scrollWidth <= node.clientWidth)).toBe(
        true,
      );
      const row = page.locator(`[data-synonym="${synonym}"]`);
      await expectInside(
        row.getByRole("button", { name: `Make "${synonym}" the main name` }),
        "Main",
      );
      await expectInside(
        row.getByRole("button", { name: `More actions for "${synonym}"` }),
      );
      await expectInside(page.getByRole("button", { name: "Delete…" }), "Delete…");
      expect(await fitsWidth()).toBe(true);

      await page.goto("/tags/kinds");
      await expect(page.getByRole("textbox", { name: "New kind name" })).toBeVisible();
      expect(await fitsWidth()).toBe(true);

      // The heading renders while the list loads; measure once the row is there.
      await page.goto("/tags/rejected");
      await expectInside(
        page.getByRole("button", { name: `Allow "${rejectedName}" again` }),
        "Allow again",
      );
      expect(await fitsWidth()).toBe(true);
      await page.setViewportSize({ width: 1280, height: 800 });
    });
  });

  // Overflow chips wrap without truncation, stay inside the viewport and scroll internally when needed; see docs/design-docs/library-ui.md.
  test.describe("カードの「+N」の一覧（041-tag-overflow-list）", () => {
    /** A 60-character unbroken name forces wrapping within the chip. */
    function longName() {
      return (
        attemptName("e2e一覧の長い名前_") +
        "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
      ).slice(0, 60);
    }

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
      await page.goto("/");
      await expect(page.getByRole("article")).toHaveCount(videoCount);

      const lastId = await page.getByRole("article").last().getAttribute("data-video-id");
      if (lastId === null) throw new Error("no data-video-id on the last card");
      const targetId = Number(lastId);
      await clearVideoTags(request, targetId);
      const tagIds = new Map<string, number>();
      for (let index = 1; index < 30; index += 1) {
        const name = attemptName(`e2e一覧${String(index).padStart(2, "0")}`);
        const tag = await createTag(request, name);
        tagIds.set(name, tag.id);
        await attachTag(request, targetId, tag.id);
      }
      const long = await createTag(request, longName());
      tagIds.set(long.name, long.id);
      await attachTag(request, targetId, long.id);

      // Thirty chips fit at normal heights; 390 x 300 requires internal scrolling to reach the last chip.
      for (const { scrolls, ...viewport } of [
        { width: 1280, height: 800, scrolls: false },
        { width: 390, height: 844, scrolls: false },
        { width: 390, height: 300, scrolls: true },
      ]) {
        await page.setViewportSize(viewport);
        await page.goto("/");
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
          name: `Filter by ${long.name}`,
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
        expect(rendered.text).toBe(long.name);
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
