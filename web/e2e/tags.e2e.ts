import { mkdir } from "node:fs/promises";
import path from "node:path";

import { expect, test, type APIRequestContext, type Page } from "@playwright/test";

// タグのうち、本物のサーバーとブラウザでしか確かめられないものだけを流す
// （docs/design-docs/web-testing.md）。作ったタグが別の動画の候補に出る・統合で
// 動画の付け替えまで通るという画面からサーバーまでの流れ 2 本と、行・一覧の実際の
// 配置、マウスを留める時間に頼る開き方、画面写真である。ほかの振る舞いは
// Vitest（src/player/VideoTags、src/tags/TagsPage*、src/library/CardTagRow）と
// Go のストアのテストが確かめる。
// 動画は run-e2e.mjs が generateTagsFixtures で作る videoCount 本
// （タグ動画A・タグ動画B・タグ動画C・タグ動画D と、本数を足すだけのタグ埋めの動画）。
// タグ埋めの動画は、1280×800 で格子の最後の行を画面の下端に
// 置くためにある（041-tag-overflow-list）。

/** generateTagsFixtures（media-fixtures.mjs）が作る動画の本数。 */
const videoCount = 16;

const screenshotDir = process.env.MDM_E2E_SCREENSHOT_DIR;

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

async function addSynonym(request: APIRequestContext, id: number, name: string) {
  const response = await request.post(`/api/tags/${String(id)}/synonyms`, {
    headers: mutationHeaders,
    data: { name },
  });
  expect(response.ok()).toBe(true);
}

async function attachTag(request: APIRequestContext, videoId: number, tagId: number) {
  const response = await request.post("/api/video-tags", {
    headers: mutationHeaders,
    data: { videoIds: [videoId], action: "add", tag: { id: tagId } },
  });
  expect(response.ok()).toBe(true);
}

/**
 * clearVideoTags は、ほかのテストが付けたタグをすべて外す。カードのタグの行は
 * 1行に収まる数が限られるので、狙ったタグを直に押せる状態にしてテストを
 * 決定的にする（「+N」の配線自体は別のテストで確かめる）。
 */
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

function addInput(page: Page) {
  return page.getByRole("combobox", { name: "Add tag" });
}

function chip(page: Page, name: string) {
  return page.locator(`[title="${name}"]`);
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

  test("1: 新しい名前で確定すると付いて表示され、別の動画の候補にも出る", async ({
    page,
  }) => {
    const a = video("タグ動画A");
    const b = video("タグ動画B");

    await page.goto(`/videos/${String(a.id)}`);
    await expect(page.getByRole("heading", { level: 1 })).toHaveText("タグ動画A");

    const input = addInput(page);
    await input.click();
    await input.fill("e2e新規タグ");
    await page.keyboard.press("Enter");

    await expect(chip(page, "e2e新規タグ")).toBeVisible();
    await expect(input).toHaveValue("");

    // 別の動画の再生画面でも、作ったタグが候補に出る。
    await page.goto(`/videos/${String(b.id)}`);
    await addInput(page).click();
    await expect(page.getByRole("option", { name: /e2e新規タグ/ })).toBeVisible();
  });

  test.describe("タグ管理画面（issue 271）", () => {
    /** tagRowByName は、名前のリンク（読み上げ名「〈名〉で絞り込んだライブラリを開く」）から行を探す。 */
    function tagRowByName(page: Page, name: string) {
      // exact: true を付けないと、Playwright の役割名の照合は大文字小文字を
      // 区別しない部分一致になり、大文字小文字だけが違う別のタグ（`Anime` と
      // `anime`、要件4）の行を取り違える。
      return page
        .getByRole("link", { name: `Open the library filtered by ${name}`, exact: true })
        .locator("xpath=ancestor::*[@data-tag-id][1]");
    }

    /**
     * rowMenuName は、幅ごとに行の右端のメニューの読み上げ名を返す。`sm`（640px）
     * 未満では行の操作が 1 つの「Actions」にまとまり、「More actions」は CSS で隠れる
     * （specs/036-tag-admin-scale/ui-design.md「Actions on touch and narrow widths」）。
     */
    function rowMenuName(width: number) {
      return width < 640 ? "Actions" : "More actions";
    }

    /**
     * revealTagRow は、名前の行が描かれるまで文書を送ってから、その行を返す。
     * 一覧は見えている行だけを描く（specs/036-tag-admin-scale/research.md R-2）
     * ので、前の試験で作ったタグが増えると、探す行が表示域の外で DOM に無い。
     * 送るたびに行が描かれるのを少し待ち、末尾まで来ても無ければ先頭から送り直す。
     * 描き直しの途中（読み直した一覧が先頭へ戻すときなど）に末尾と見誤って
     * やめないよう、回数ではなく時間で区切る。
     */
    async function revealTagRow(page: Page, name: string) {
      const row = tagRowByName(page, name);
      await expect(
        page.getByRole("link", { name: /^Open the library filtered by / }).first(),
      ).toBeVisible();
      await page.evaluate(() => window.scrollTo(0, 0));
      // 行が描かれたあとでも、一覧が測り直して描く範囲を動かすと行は DOM から外れる。
      // 表示域へ送るところまでを 1 回の試みに含め、外れたら送り直す。
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
      // 仮のタグは外部連携 API の tentative でだけ作れる（specs/031-tentative-tags）。
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

      // 名前の列が最も狭くなる 360px でも確かめる（ui-design.md「Responsive behaviour」）。
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
          // 目印は名前の後ろに、同じ行で並ぶ（間の幅は見ない）。
          const linkBox = await link.boundingBox();
          const markBox = await mark.boundingBox();
          if (linkBox === null || markBox === null) throw new Error("no layout");
          expect(markBox.x).toBeGreaterThanOrEqual(linkBox.x + linkBox.width);
          expect(markBox.y).toBeLessThan(linkBox.y + linkBox.height);
          expect(markBox.y + markBox.height).toBeGreaterThan(linkBox.y);
          // 長い名前でも操作は行の中に収まる（`sm` 未満では「Actions」1 つ）。
          const rowBox = await row.boundingBox();
          const moreBox = await row
            .getByRole("button", { name: rowMenuName(width) })
            .boundingBox();
          if (rowBox === null || moreBox === null) throw new Error("no layout");
          expect(moreBox.x + moreBox.width).toBeLessThanOrEqual(rowBox.x + rowBox.width);
        }
        // 長い名前は省略される。
        await revealTagRow(page, longName);
        const longLink = tagRowByName(page, longName).getByRole("link", {
          name: `Open the library filtered by ${longName}`,
          exact: true,
        });
        expect(
          await longLink.evaluate((node) => node.scrollWidth > node.clientWidth),
        ).toBe(true);
        // 横スクロールは出ない。
        expect(
          await page.evaluate(
            () => document.documentElement.scrollWidth <= window.innerWidth,
          ),
        ).toBe(true);
      }
      await page.setViewportSize({ width: 1280, height: 800 });

      // 短い名前の行で、名前と目印より右の空白を押す。
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
      // 4 桁の件数と 4 桁の却下した名前で、見出しとタブが最も長くなる形を作る
      // （ui-design.md「Header」「Tabs」「Responsive behaviour」）。
      const createdAt = "2026-01-01T00:00:00Z";
      const items = Array.from({ length: 1000 }, (_, index) => ({
        id: 900000 + index,
        name: `e2e狭い幅${String(index).padStart(4, "0")}`,
        synonyms: [],
        videoCount: 0,
        tentative: true,
        // `kind` は必須（null は種類なし）。欠けると種類の名札が描けず画面が落ちる。
        kind: null,
        createdAt,
      }));
      // ページで読む画面はクエリ（sort・limit・tentative）を付けて送る。
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
        // 「Tentative only」が効いた URL で開く（ui-design.md「URL state」）。
        await page.goto("/tags?tentative=1");
        const count = page.getByRole("status").filter({ hasText: "1,000 of 1,000 tags" });
        await expect(count).toBeVisible();
        const tab = page.getByRole("tab", { name: /^Rejected names/ });
        await expect(tab).toBeVisible();
        await expect(tab).toContainText("1,000");
        const tabBox = await tab.boundingBox();
        if (tabBox === null) throw new Error("no layout");
        // 件数は 1 行のまま（折り返すと行の箱が 2 つ以上になる）。
        expect(
          await count.evaluate((element) => {
            const range = document.createRange();
            range.selectNodeContents(element);
            return new Set(
              [...range.getClientRects()].map((rect) => Math.round(rect.top)),
            ).size;
          }),
        ).toBe(1);
        // タブは画面の中にある。
        expect(tabBox.x + tabBox.width).toBeLessThanOrEqual(width);
        // 横スクロールは出ない。
        expect(
          await page.evaluate(
            () => document.documentElement.scrollWidth <= window.innerWidth,
          ),
        ).toBe(true);
        // タブは押せ、本文に却下した名前が出る。
        await tab.click();
        await expect(
          page.getByRole("tabpanel", { name: /^Rejected names/ }).getByRole("table"),
        ).toBeVisible();
        // 3 つ目の「Kinds」は、収まらなければタブの並びの中で送って押せ、画面は横に送れない。
        const kinds = page.getByRole("tab", { name: /^Kinds/ });
        await kinds.click();
        await expect(page).toHaveURL(/[?&]tab=kinds/);
        const kindsBox = await kinds.boundingBox();
        if (kindsBox === null) throw new Error("no layout");
        expect(kindsBox.x + kindsBox.width).toBeLessThanOrEqual(width);
        expect(
          await page.evaluate(
            () => document.documentElement.scrollWidth <= window.innerWidth,
          ),
        ).toBe(true);
        // キーボードで移ったタブのフォーカスの輪（外へ 4px）は、送る帯に切られない。
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
        // URL から開いたとき、戻ったときも、選んでいる「Kinds」は帯の見える位置にある。
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
        await page.goBack();
        await expect(page).toHaveURL(/[?&]tab=kinds/);
        await kindsInView();
      }
      await page.setViewportSize({ width: 1280, height: 800 });
    });

    test("タグ管理画面の画像を撮る", async ({ page, request }) => {
      test.skip(screenshotDir === undefined, "MDM_E2E_SCREENSHOT_DIR is not set");
      const dir = screenshotDir ?? "";
      await mkdir(dir, { recursive: true });
      const anime = await createTag(request, "e2e画像Anime");
      await addSynonym(request, anime.id, "e2e画像アニメ");
      await createTag(request, "e2e画像旅行");
      for (const width of [360, 1280]) {
        await page.setViewportSize({ width, height: 800 });
        await page.goto("/tags");
        await revealTagRow(page, "e2e画像Anime");
        await expect(tagRowByName(page, "e2e画像Anime")).toBeVisible();
        await page.screenshot({ path: path.join(dir, `tags-list-${String(width)}.png`) });

        await revealTagRow(page, "e2e画像旅行");
        await tagRowByName(page, "e2e画像旅行")
          .getByRole("button", { name: rowMenuName(width) })
          .click();
        await page.getByRole("menuitem", { name: "Merge into another tag…" }).click();
        const dialog = page.getByRole("dialog", { name: 'Merge "e2e画像旅行"' });
        await dialog.getByRole("combobox", { name: "Tag to merge into" }).fill("e2e画像");
        await page.getByRole("option", { name: /e2e画像Anime/ }).click();
        await page.screenshot({
          path: path.join(dir, `tags-merge-${String(width)}.png`),
        });
        await page.keyboard.press("Escape");

        await revealTagRow(page, "e2e画像Anime");
        if (width < 640) {
          // `sm` 未満では「シノニム」も「Actions」のメニューの項目になる。
          await tagRowByName(page, "e2e画像Anime")
            .getByRole("button", { name: rowMenuName(width) })
            .click();
          await page.getByRole("menuitem", { name: "Synonyms" }).click();
        } else {
          await tagRowByName(page, "e2e画像Anime")
            .getByRole("button", { name: "Synonyms" })
            .click();
        }
        const synonyms = page.getByRole("dialog", { name: 'Synonyms of "e2e画像Anime"' });
        await synonyms.getByRole("textbox", { name: "Add synonym" }).fill("e2e画像旅行");
        await page.keyboard.press("Enter");
        await expect(synonyms.getByText(/is a tag on/)).toBeVisible();
        await page.screenshot({
          path: path.join(dir, `tags-synonym-merge-${String(width)}.png`),
        });
        await synonyms.getByRole("button", { name: "Back" }).click();
        await page.keyboard.press("Escape");
      }
    });

    test("12: 統合すると統合元が付いていた動画すべてに統合先が付き、統合元は一覧から消えて統合先のシノニムになる", async ({
      page,
      request,
    }) => {
      const x = await createTag(request, "e2e管理統合元X");
      const y = await createTag(request, "e2e管理統合先Y");
      const a = video("タグ動画A");
      const b = video("タグ動画B");
      await clearVideoTags(request, a.id);
      await clearVideoTags(request, b.id);
      await attachTag(request, a.id, x.id);
      await attachTag(request, b.id, x.id);
      await attachTag(request, b.id, y.id);

      await page.goto("/tags");
      await revealTagRow(page, "e2e管理統合元X");
      const row = tagRowByName(page, "e2e管理統合元X");
      await row.getByRole("button", { name: "More actions" }).click();
      await page.getByRole("menuitem", { name: "Merge into another tag…" }).click();

      const dialog = page.getByRole("dialog", { name: 'Merge "e2e管理統合元X"' });
      await dialog
        .getByRole("combobox", { name: "Tag to merge into" })
        .fill("e2e管理統合先Y");
      await page.getByRole("option", { name: /e2e管理統合先Y/ }).click();
      await expect(
        dialog.getByText(
          'The 2 videos tagged "e2e管理統合元X" get the tag "e2e管理統合先Y". ' +
            '"e2e管理統合元X" and its synonyms become synonyms of "e2e管理統合先Y", ' +
            'and "e2e管理統合元X" leaves the tag list. This can\'t be undone.',
        ),
      ).toBeVisible();
      await dialog.getByRole("button", { name: "Merge" }).click();

      await expect(
        page.getByText('Merged "e2e管理統合元X" into "e2e管理統合先Y"'),
      ).toBeVisible();
      await expect(tagRowByName(page, "e2e管理統合元X")).toHaveCount(0);
      const targetRow = tagRowByName(page, "e2e管理統合先Y");
      await expect(targetRow).toContainText("Synonyms: e2e管理統合元X");
      // 両方付いていた B には Y が1つだけ付く（重複して並ばない）。
      await expect(targetRow).toContainText("2 videos");

      // A（X だけが付いていた）にも Y が付く。
      await page.goto(`/videos/${String(a.id)}`);
      await expect(chip(page, "e2e管理統合先Y")).toBeVisible();
      await expect(chip(page, "e2e管理統合元X")).toHaveCount(0);

      // 統合後、再生画面で X と入力して確定すると Y が付く（受け入れ条件12）。
      const c = video("タグ動画C");
      await clearVideoTags(request, c.id);
      await page.goto(`/videos/${String(c.id)}`);
      const input = addInput(page);
      await input.click();
      await input.fill("e2e管理統合元X");
      await page.keyboard.press("Enter");
      await expect(chip(page, "e2e管理統合先Y")).toBeVisible();
    });

    test("B3: 1280幅で統合先の候補を開くと、8行すべてが見える（ui-design.mdは最大8行）", async ({
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

      // 8行すべてが、窓の overflow によって切り取られずに見える
      // （MergeTagDialog.tsx の overflow-y-auto がこの候補の一覧まで
      // 覆っていたときは、ここが見えないまま timeout していた）。
      for (const name of names) {
        await expect(page.getByRole("option", { name: new RegExp(name) })).toBeVisible();
      }

      // 最後の行（8番目）を実際に選べる（クリックが素通りしないことも確かめる）。
      await page.getByRole("option", { name: new RegExp(names[7]!) }).click();
      await expect(
        dialog.getByText(new RegExp(`get the tag "${names[7]!}"`)),
      ).toBeVisible();
    });
  });

  // specs/041-tag-overflow-list: カードの「+N」の一覧は、隠れたタグを折り返す
  // チップで並べ、画面の中に収まり、収まらない分は一覧の中でスクロールし、長い
  // 名前を省略しない（親 Issue #813 の受け入れ条件 1・5 と、画面の下端に近い
  // カードの Edge Case）。マウスを「+N」に留めると開き、押しても開く（受け入れ
  // 条件 2・3・4）。
  test.describe("カードの「+N」の一覧（041-tag-overflow-list）", () => {
    /** 60文字の、空白を持たない名前。一覧の内幅より広いので、チップの中で折り返す。 */
    const longName = (
      "e2e一覧の長い名前_" + "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
    ).slice(0, 60);

    /** box が画面の中に、Radix の collisionPadding（8px）を空けて収まるかを確かめる。 */
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
      // 格子の最後のカード（どの幅でも DOM の順は同じ）に30個のタグを付ける。
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

      // 1280×800 と 390×844 では30個の一覧が画面に収まり、並びはスクロールしない。
      // 低い 390×300 では収まらず、並びの中のスクロールで最後のタグへ届くことを
      // 確かめる（要件3）。
      for (const { scrolls, ...viewport } of [
        { width: 1280, height: 800, scrolls: false },
        { width: 390, height: 844, scrolls: false },
        { width: 390, height: 300, scrolls: true },
      ]) {
        await page.setViewportSize(viewport);
        await page.goto("/library");
        await expect(page.getByRole("article")).toHaveCount(videoCount);
        const card = page.locator(`article[data-video-id="${String(targetId)}"]`);
        // 格子の最後の行を画面の下端まで送る（最後のカードを見せたうえで、
        // その先へも回し切る）。
        await card.scrollIntoViewIfNeeded();
        await page.mouse.move(viewport.width / 2, viewport.height / 2);
        await page.mouse.wheel(0, 10_000);
        const overflow = card.getByRole("button", { name: /^Show \d+ more tags?$/ });
        await expect(overflow).toBeVisible();
        const moreBox = await overflow.boundingBox();
        if (moreBox === null) throw new Error("no layout for +N");
        if (viewport.width === 1280) {
          // 「+N」は画面の下半分にあり、下には30個の一覧を置く余地が無い。
          expect(moreBox.y).toBeGreaterThan(viewport.height / 2);
        }

        await overflow.click();
        const dialog = page.getByRole("dialog");
        await expect(dialog).toBeVisible();
        // 開くときの拡大の動きが済んでから箱を測る。
        await dialog.evaluate((element) =>
          Promise.all(element.getAnimations().map((animation) => animation.finished)),
        );
        // 一覧の高さは、Radix が知らせる残りの高さで抑えられている（R-2）。
        expect(
          await dialog.evaluate((element) => getComputedStyle(element).maxHeight),
        ).toMatch(/px$/);
        const dialogBox = await dialog.boundingBox();
        if (dialogBox === null) throw new Error("no layout for the list");
        expectInsideViewport(dialogBox, viewport);
        if (viewport.width === 1280) {
          // 下端の近くのカードでは、一覧は「+N」の上か横に開く（Edge Case）。
          const above = dialogBox.y + dialogBox.height <= moreBox.y;
          const beside =
            dialogBox.x >= moreBox.x + moreBox.width ||
            dialogBox.x + dialogBox.width <= moreBox.x;
          expect(above || beside).toBe(true);
        }

        // 60文字の名前は … なしで全文、複数行で描かれる（受け入れ条件5）。
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

        if (screenshotDir !== undefined && !scrolls) {
          await mkdir(screenshotDir, { recursive: true });
          await dialog.evaluate((element) => element.querySelector("ul")?.scrollTo(0, 0));
          await page.screenshot({
            path: path.join(
              screenshotDir,
              `tag-overflow-list-${String(viewport.width)}.png`,
            ),
          });
        }

        // 一覧をスクロールすると最後のタグが見え、押すと絞り込まれて一覧が閉じる
        // （受け入れ条件1）。
        const chips = dialog.getByRole("button");
        await expect(chips).toHaveCount(30 - (await visibleChipCount(card)));
        const lastChip = chips.last();
        const list = dialog.getByRole("list");
        if (scrolls) {
          // 並びは板の高さを超え、最後のタグは並びをスクロールするまで見えない。
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
        const lastName = await lastChip.getAttribute("title");
        const lastTagId = lastName === null ? undefined : tagIds.get(lastName);
        if (lastTagId === undefined) throw new Error(`unknown tag: ${String(lastName)}`);
        await lastChip.click();
        await expect(page).toHaveURL(new RegExp(`tag=${String(lastTagId)}(&|$)`));
        await expect(page.getByRole("dialog")).toHaveCount(0);
        await expect(page.getByRole("article")).toHaveCount(1);
      }
    });

    /**
     * cardWithHiddenTags は、格子の最初のカードのタグを付け直し、短い名前のタグを
     * 12個付ける。行に収まらない分が「+N」に入る。名前は prefix で他のテストと分ける。
     */
    async function cardWithHiddenTags(
      page: Page,
      request: APIRequestContext,
      prefix: string,
    ) {
      await page.goto("/library");
      await expect(page.getByRole("article")).toHaveCount(videoCount);
      const firstId = await page
        .getByRole("article")
        .first()
        .getAttribute("data-video-id");
      if (firstId === null) throw new Error("no data-video-id on the first card");
      const targetId = Number(firstId);
      await clearVideoTags(request, targetId);
      const tagIds = new Map<string, number>();
      for (let index = 1; index <= 12; index += 1) {
        const name = `${prefix}${String(index).padStart(2, "0")}`;
        const tag = await createTag(request, name);
        tagIds.set(name, tag.id);
        await attachTag(request, targetId, tag.id);
      }
      await page.reload();
      const card = page.locator(`article[data-video-id="${String(targetId)}"]`);
      const overflow = card.getByRole("button", { name: /^Show \d+ more tags?$/ });
      await expect(overflow).toBeVisible();
      return { card, overflow, tagIds };
    }

    /** center は要素の箱の中央の座標を返す。 */
    async function center(locator: ReturnType<Page["locator"]>) {
      const box = await locator.boundingBox();
      if (box === null) throw new Error("no layout");
      return { x: box.x + box.width / 2, y: box.y + box.height / 2 };
    }

    test("2・3: 1280×800 でマウスを「+N」に留めるとクリックなしで開き、一覧へ移っても開いたまま、横切っただけでは開かない", async ({
      page,
      request,
    }) => {
      test.setTimeout(120_000);
      await page.setViewportSize({ width: 1280, height: 800 });
      const { overflow, tagIds } = await cardWithHiddenTags(page, request, "e2e留め");
      const dialog = page.getByRole("dialog");
      await page.mouse.move(5, 795);
      // hover の見た目は、文字色が text-muted-foreground から text-foreground に
      // 変わる（面の bg-accent は secondary と同じ値）。
      const restColor = await overflow.evaluate(
        (element) => getComputedStyle(element).color,
      );

      // 横切って 400ms 未満で離れると開かない（受け入れ条件3）。留まった時間は
      // テストの実行側の往復ではなく、ブラウザが受けた pointerenter から
      // pointerleave までで測り、一覧が一瞬でも現れたかを MutationObserver で見る。
      await overflow.evaluate((element) => {
        const probe = { enteredAt: -1, leftAt: -1, dialogSeen: false };
        (window as unknown as { __crossProbe: typeof probe }).__crossProbe = probe;
        element.addEventListener("pointerenter", () => {
          probe.enteredAt = performance.now();
        });
        element.addEventListener("pointerleave", () => {
          probe.leftAt = performance.now();
        });
        new MutationObserver(() => {
          if (document.querySelector('[role="dialog"]') !== null) probe.dialogSeen = true;
        }).observe(document.body, { childList: true, subtree: true });
      });
      const target = await center(overflow);
      await page.mouse.move(target.x, target.y);
      await page.mouse.move(5, 795);
      await page.waitForTimeout(800);
      await expect(dialog).toHaveCount(0);
      const crossing = await page.evaluate(
        () =>
          (
            window as unknown as {
              __crossProbe: { enteredAt: number; leftAt: number; dialogSeen: boolean };
            }
          ).__crossProbe,
      );
      expect(crossing.enteredAt).toBeGreaterThanOrEqual(0);
      expect(crossing.leftAt).toBeGreaterThanOrEqual(crossing.enteredAt);
      expect(crossing.leftAt - crossing.enteredAt).toBeLessThan(400);
      expect(crossing.dialogSeen).toBe(false);

      // 留めるとクリックなしで開き、フォーカスは動かない（受け入れ条件2、R-1）。
      const focusedBefore = await page.evaluateHandle(() => document.activeElement);
      await page.mouse.move(target.x, target.y);
      await expect(dialog).toBeVisible();
      expect(
        await page.evaluate((before) => document.activeElement === before, focusedBefore),
      ).toBe(true);

      // 一覧へ移る間も、一覧の上でも開いたまま。開いている間「+N」は hover の見た目を保つ。
      const firstChip = dialog.getByRole("button").first();
      const chipCenter = await center(firstChip);
      await page.mouse.move(chipCenter.x, chipCenter.y, { steps: 10 });
      await page.waitForTimeout(500);
      await expect(dialog).toBeVisible();
      await expect(overflow).toHaveAttribute("data-state", "open");
      expect(
        await overflow.evaluate((element) => getComputedStyle(element).color),
      ).not.toBe(restColor);
      if (screenshotDir !== undefined) {
        await mkdir(screenshotDir, { recursive: true });
        await page.screenshot({
          path: path.join(screenshotDir, "tag-overflow-hover-1280.png"),
        });
      }

      // 一覧のチップを押すと、そのタグで絞り込まれて一覧が閉じる（受け入れ条件2）。
      const name = await firstChip.getAttribute("title");
      const tagId = name === null ? undefined : tagIds.get(name);
      if (tagId === undefined) throw new Error(`unknown tag: ${String(name)}`);
      await firstChip.click();
      await expect(page).toHaveURL(new RegExp(`tag=${String(tagId)}(&|$)`));
      await expect(dialog).toHaveCount(0);
    });

    /** visibleChipCount は、カードの行に直に出ているタグのチップの数（「+N」を除く）。 */
    async function visibleChipCount(card: ReturnType<Page["locator"]>) {
      return card
        .getByRole("list", { name: "Tags", exact: true })
        .getByRole("button", { name: /^Filter by / })
        .count();
    }
  });
});
