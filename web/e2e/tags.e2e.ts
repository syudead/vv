import { mkdir } from "node:fs/promises";
import path from "node:path";

import { expect, test, type APIRequestContext, type Page } from "@playwright/test";

// 再生画面のタグ（issue 268、親 Issue #193 の受け入れ条件 1・2・6・13）、
// ライブラリのカードのタグ・タグでの絞り込み（issue 269、受け入れ条件 5・7・8・19・20）、
// 選択バーの一括操作・すべて選択（issue 270、受け入れ条件 3・4）を実ブラウザに通す。
// 動画は run-e2e.mjs が generateTagsFixtures で作る 4 本
// （タグ動画A・タグ動画B・タグ動画C・タグ動画D）。タグ動画Cはタグを持たない対照区
// （issue 269 受け入れ条件7）で、issue 270 のテストでは触らない。

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

async function renameTag(request: APIRequestContext, id: number, name: string) {
  const response = await request.patch(`/api/tags/${String(id)}`, {
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
    expect(page.items).toHaveLength(4);
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

  test("2: 外すボタンでタグが消え、再読み込みしても消えたまま", async ({
    page,
    request,
  }) => {
    const a = video("タグ動画A");
    const tag = await createTag(request, "e2e外すタグ");
    const attach = await request.post("/api/video-tags", {
      headers: mutationHeaders,
      data: { videoIds: [a.id], action: "add", tag: { id: tag.id } },
    });
    expect(attach.ok()).toBe(true);

    await page.goto(`/videos/${String(a.id)}`);
    const removeButton = page.getByRole("button", {
      name: "Remove e2e外すタグ from this video",
    });
    await expect(removeButton).toBeVisible();
    await removeButton.click();
    await expect(chip(page, "e2e外すタグ")).toHaveCount(0);

    await page.reload();
    await expect(page.getByRole("heading", { level: 1 })).toBeVisible();
    await expect(chip(page, "e2e外すタグ")).toHaveCount(0);
  });

  test("6: `anime`はシノニム未登録なら別タグになり、無効な入力は確定できず理由が出る", async ({
    page,
    request,
  }) => {
    await createTag(request, "Anime");
    const a = video("タグ動画A");
    await page.goto(`/videos/${String(a.id)}`);
    const input = addInput(page);

    // 空白だけは Enter で確定できない。
    await input.click();
    await input.fill("   ");
    await page.keyboard.press("Enter");
    await expect(page.getByText("Enter a name")).toBeVisible();

    // 改行を含む貼り付けは取り込まない。
    await input.fill("");
    await input.evaluate((element) => {
      const data = new DataTransfer();
      data.setData("text/plain", "旅行\n2024");
      element.dispatchEvent(
        new ClipboardEvent("paste", {
          clipboardData: data,
          bubbles: true,
          cancelable: true,
        }),
      );
    });
    await expect(page.getByText("Line breaks and tabs aren't allowed")).toBeVisible();
    await expect(input).toHaveValue("");

    // 101 文字は確定できない。
    await input.fill("あ".repeat(101));
    await expect(
      page.getByText("Use 100 characters or fewer (currently 101)"),
    ).toBeVisible();
    await page.keyboard.press("Enter");
    await expect(chip(page, "あ".repeat(101))).toHaveCount(0);

    // `anime`はシノニム未登録の`Anime`とは別のタグとして作られる。
    await input.fill("");
    await input.fill("anime");
    await page.keyboard.press("Enter");
    await expect(chip(page, "anime")).toBeVisible();
    await expect(chip(page, "Anime")).toHaveCount(0);
  });

  test("13: シノニム登録済みの名前を入力すると、元のタグが付く", async ({
    page,
    request,
  }) => {
    const tag = await createTag(request, "e2eシノニム元");
    await addSynonym(request, tag.id, "e2eシノニムべつ");
    const b = video("タグ動画B");

    await page.goto(`/videos/${String(b.id)}`);
    const input = addInput(page);
    await input.click();
    await input.fill("e2eシノニムべつ");
    await page.keyboard.press("Enter");

    await expect(chip(page, "e2eシノニム元")).toBeVisible();
    await expect(chip(page, "e2eシノニムべつ")).toHaveCount(0);
  });

  test("矢印キーで候補を選び、Enterで付けられる（キーボードだけ）", async ({
    page,
    request,
  }) => {
    await createTag(request, "e2e候補タグ");
    const a = video("タグ動画A");
    await page.goto(`/videos/${String(a.id)}`);

    const input = addInput(page);
    await input.click();
    // 「候補」に絞ると、この動画にはまだ無いこのタグが先頭の候補に出る。
    // 完全一致が無いので、その後に作成の行が続く。
    await input.fill("候補");
    await expect(page.getByRole("option").first()).toHaveText(/e2e候補タグ/);
    await page.keyboard.press("ArrowDown");
    await page.keyboard.press("Enter");

    await expect(chip(page, "e2e候補タグ")).toBeVisible();

    // 一覧が閉じているときの Esc は入力を空にする（ui-design.md「Add input」、B1）。
    await input.fill("捨てる文字");
    await page.keyboard.press("Escape"); // 1 回目: 開いている一覧を閉じる
    await page.keyboard.press("Escape"); // 2 回目: 入力を空にする
    await expect(input).toHaveValue("");
  });

  test.describe("ライブラリのカードのタグ・タグでの絞り込み（issue 269）", () => {
    function activeTagChip(page: Page, name: string) {
      return page
        .getByRole("list", { name: "Tags in the filter" })
        .getByRole("button", { name: `Remove the filter for ${name}` });
    }

    /**
     * pressCardTag は、カードのタグを押して絞り込む。ほかのテストで付けたタグが
     * 積み重なって1行に収まらないことがあるので、直に見えなければ「+N」を開いて
     * そこから押す（ui-design.md「Overflow」: 「+N」のチップを押しても同じく絞り込む）。
     */
    async function pressCardTag(page: Page, videoId: number, name: string) {
      const card = page.locator(`[data-video-id="${String(videoId)}"]`);
      const direct = card.getByRole("button", { name: `Filter by ${name}`, exact: true });
      try {
        // 直に見えていれば、それを押す。レイアウトの計測（B2）が済むまでの
        // 短い間は「+N」に入っていることもあるので、.count() の1回きりの
        // 判定ではなく、Playwright の自動待機に判断を委ねる。
        await direct.click({ timeout: 5000 });
        return;
      } catch {
        // 直には無い（本当に収まらず「+N」の中）。
      }
      await card.getByRole("button", { name: /^Show \d+ more tags?$/ }).click();
      await page
        .getByRole("dialog")
        .getByRole("button", { name: `Filter by ${name}`, exact: true })
        .click();
    }

    test("5: カードの題名の下にタグが出て、追加日時・ファイルサイズ・コーデックは出ない", async ({
      page,
      request,
    }) => {
      const tag = await createTag(request, "e2eカード表示");
      const a = video("タグ動画A");
      await attachTag(request, a.id, tag.id);

      await page.goto("/");
      const card = page.locator(`[data-video-id="${String(a.id)}"]`);
      // 追加日時・ファイルサイズ・コーデックの行の代わりにタグの行が出る。
      await expect(card.getByRole("list", { name: "Tags", exact: true })).toBeVisible();
      // 今のメタ情報の行（追加日時 · ファイルサイズ · コーデック）は区切りの
      // "·" を持つが、タグの行はそれに置き換わるので出ない。
      await expect(card.getByText("·")).toHaveCount(0);
    });

    test("5: 1行に収まらないタグは +N にまとまり、押すと残りが見える", async ({
      page,
      request,
    }) => {
      // 動画Aは後続のテストで直接タグを押すので、動画Bにだけ多数のタグを付ける
      // （収まりきらない状態を作っても、ほかのテストの「見えているチップを押す」
      // 前提を崩さないため）。
      const b = video("タグ動画B");
      for (let index = 0; index < 10; index += 1) {
        const tag = await createTag(request, `e2eオーバーフロー${String(index)}`);
        await attachTag(request, b.id, tag.id);
      }

      await page.goto("/");
      const overflow = page
        .locator(`[data-video-id="${String(b.id)}"]`)
        .getByRole("button", { name: /^Show \d+ more tags?$/ });
      await expect(overflow).toBeVisible();
      await overflow.click();
      await expect(page.getByRole("dialog")).toBeVisible();
      await expect(
        page.getByRole("dialog").getByText("e2eオーバーフロー9"),
      ).toBeVisible();
    });

    test("7: カードのタグを押すと絞り込まれ、上の行から1回で外せる。再生画面のタグを押すとその1つで絞り込んだ一覧が開く", async ({
      page,
      request,
    }) => {
      const tag = await createTag(request, "e2e旅行");
      const a = video("タグ動画A");
      const b = video("タグ動画B");
      await clearVideoTags(request, a.id);
      await clearVideoTags(request, b.id);
      await attachTag(request, a.id, tag.id);
      await attachTag(request, b.id, tag.id);

      await page.goto("/");
      // 絞り込み前は、タグの無いタグ動画C・Dも含めて4件出る。
      await expect(page.getByRole("article")).toHaveCount(4);

      await pressCardTag(page, a.id, "e2e旅行");
      await expect(page).toHaveURL(new RegExp(`tag=${String(tag.id)}(&|$)`));
      // タグの付いた両方が残り、検索欄は変わらない。件数も正しい。
      await expect(page.getByRole("article")).toHaveCount(2);
      await expect(activeTagChip(page, "e2e旅行")).toBeVisible();
      await expect(page.getByRole("searchbox", { name: "Search videos" })).toHaveValue(
        "",
      );
      await expect(page.getByRole("status", { name: "Search results" })).toHaveText(
        "2 items",
      );

      // タグの絞り込みは、題名にその文字列を含むだけのタグの無い動画を出さない
      // （受け入れ条件7、親 Issue「タグの無い動画」の要求を満たさない例）。ここでは
      // タグ動画Cの題名そのものを名前に持つタグを作り、Aにだけ付ける。
      const titleLikeTag = await createTag(request, "タグ動画C");
      await attachTag(request, a.id, titleLikeTag.id);
      await activeTagChip(page, "e2e旅行").click();
      await expect(page).not.toHaveURL(/tag=/);
      await pressCardTag(page, a.id, "タグ動画C");
      await expect(page.getByRole("article")).toHaveCount(1);
      await expect(page.getByRole("link", { name: "タグ動画A" })).toBeVisible();
      await expect(page.getByRole("link", { name: "タグ動画C" })).toHaveCount(0);
      await activeTagChip(page, "タグ動画C").click();
      await expect(page).not.toHaveURL(/tag=/);

      await pressCardTag(page, a.id, "e2e旅行");
      await expect(page.getByRole("article")).toHaveCount(2);

      // 一覧の上のチップを1回押すと外れ、押す前の一覧（タグ動画C・Dを含む4件）に戻る。
      await activeTagChip(page, "e2e旅行").click();
      await expect(page).not.toHaveURL(/tag=/);
      await expect(page.getByRole("article")).toHaveCount(4);

      // 再生画面でタグを押すと、そのタグ1つで絞り込んだライブラリ一覧が開く。
      await page.goto(`/videos/${String(a.id)}`);
      await page.getByRole("link", { name: "Filter by e2e旅行" }).click();
      await expect(page).toHaveURL(
        new RegExp(`^http://127\\.0\\.0\\.1:15173/\\?tag=${String(tag.id)}$`),
      );
      await expect(page.getByRole("article")).toHaveCount(2);
    });

    test("8: 2つのタグを AND で絞り込み、1つだけ外すと元の絞り込みに戻る。開き直しても同じ絞り込みになる", async ({
      page,
      request,
    }) => {
      const travel = await createTag(request, "e2eAND旅行");
      const year = await createTag(request, "e2eAND2024");
      const a = video("タグ動画A");
      const b = video("タグ動画B");
      await clearVideoTags(request, a.id);
      await clearVideoTags(request, b.id);
      await attachTag(request, a.id, travel.id);
      await attachTag(request, b.id, travel.id);
      await attachTag(request, a.id, year.id);

      await page.goto("/");
      await pressCardTag(page, a.id, "e2eAND旅行");
      await expect(page.getByRole("article")).toHaveCount(2);

      await pressCardTag(page, a.id, "e2eAND2024");
      await expect(page).toHaveURL(new RegExp(`tag=${String(travel.id)}`));
      await expect(page).toHaveURL(new RegExp(`tag=${String(year.id)}`));
      // 両方持つ A だけが残る。
      await expect(page.getByRole("article")).toHaveCount(1);
      await expect(page.getByRole("link", { name: "タグ動画A" })).toBeVisible();

      // 2024 だけを外すと、旅行だけの絞り込みに戻る（両方見える）。
      await activeTagChip(page, "e2eAND2024").click();
      await expect(page).not.toHaveURL(new RegExp(`tag=${String(year.id)}`));
      await expect(page.getByRole("article")).toHaveCount(2);

      // タグの絞り込みに加えて、検索欄の題名検索がさらに絞り込む（受け入れ条件8）。
      const box = page.getByRole("searchbox", { name: "Search videos" });
      await box.fill("タグ動画A");
      await expect(page.getByRole("article")).toHaveCount(1);
      await expect(page.getByRole("link", { name: "タグ動画A" })).toBeVisible();
      // 検索語を消すと、タグの絞り込みだけの2件に戻る。
      await box.fill("");
      await expect(page.getByRole("article")).toHaveCount(2);

      // 動画を開いて戻っても、再読み込みしても、新しいタブで開いても同じ絞り込みになる。
      const url = page.url();
      await page.getByRole("link", { name: "タグ動画A" }).click();
      await expect(page.getByRole("heading", { level: 1 })).toHaveText("タグ動画A");
      await page.goBack();
      await expect(page).toHaveURL(url);
      await expect(page.getByRole("article")).toHaveCount(2);

      await page.reload();
      await expect(page.getByRole("article")).toHaveCount(2);

      const context = page.context();
      const other = await context.newPage();
      await other.goto(url);
      await expect(other.getByRole("article")).toHaveCount(2);
      await other.close();
    });

    test("19・20: 検索欄はタグ名・シノニムからも探せ、-タグ名は除外する", async ({
      page,
      request,
    }) => {
      const searchable = await createTag(request, "e2e検索専用タグ");
      await addSynonym(request, searchable.id, "e2eシノニム検索");
      const a = video("タグ動画A");
      video("タグ動画B");
      await attachTag(request, a.id, searchable.id);

      await page.goto("/");
      const box = page.getByRole("searchbox", { name: "Search videos" });

      // 題名に含まないタグ名で検索すると、そのタグの付いた動画が出る。
      await box.fill("e2e検索専用タグ");
      await expect(page.getByRole("link", { name: "タグ動画A" })).toBeVisible();
      await expect(page.getByRole("link", { name: "タグ動画B" })).toHaveCount(0);

      // シノニムでも当たる。
      await box.fill("e2eシノニム検索");
      await expect(page.getByRole("link", { name: "タグ動画A" })).toBeVisible();

      // -タグ名は、そのタグの付いた動画を除外する。
      await box.fill("-e2e検索専用タグ");
      await expect(page.getByRole("link", { name: "タグ動画B" })).toBeVisible();
      await expect(page.getByRole("link", { name: "タグ動画A" })).toHaveCount(0);

      await box.fill("");

      // フォルダ画面の検索欄も同じ規則で探す（受け入れ条件19、要件10）。
      await page.goto("/folders");
      const folderBox = page.getByRole("searchbox", {
        name: "Search videos in all folders",
      });
      await folderBox.fill("e2e検索専用タグ");
      await expect(page.getByRole("link", { name: /^タグ動画A/ })).toBeVisible();
      await expect(page.getByRole("link", { name: /^タグ動画B/ })).toHaveCount(0);

      await folderBox.fill("-e2e検索専用タグ");
      await expect(page.getByRole("link", { name: /^タグ動画B/ })).toBeVisible();
      await expect(page.getByRole("link", { name: /^タグ動画A/ })).toHaveCount(0);

      await folderBox.fill("");

      // タグを改名すると、次の検索から新しい名前で当たる（受け入れ条件20）。
      await renameTag(request, searchable.id, "e2e改名後タグ");
      await page.goto("/");
      await box.fill("e2e改名後タグ");
      await expect(page.getByRole("link", { name: "タグ動画A" })).toBeVisible();
      await expect(page.getByRole("link", { name: "タグ動画B" })).toHaveCount(0);
      // 古い名前ではもう当たらない。
      await box.fill("e2e検索専用タグ");
      await expect(page.getByRole("link", { name: "タグ動画A" })).toHaveCount(0);
      await box.fill("");
    });
  });

  test.describe("選択バーの一括操作・すべて選択（issue 270）", () => {
    function checkbox(page: Page, title: string) {
      return page.getByRole("checkbox", { name: `Select "${title}"` });
    }

    test("3: 3本を選び選択バーからタグを付けると全部に付く。外す候補で一部にしか付いていないタグが分かる", async ({
      page,
      request,
    }) => {
      const a = video("タグ動画A");
      const b = video("タグ動画B");
      // タグ動画Cは issue 269 の受け入れ条件7の対照区（タグを持たない）なので
      // ここでは触らず、選択には別の動画（タグ動画D）を使う。
      const d = video("タグ動画D");
      await clearVideoTags(request, a.id);
      await clearVideoTags(request, b.id);
      await clearVideoTags(request, d.id);
      const solo = await createTag(request, "e2e一部タグ");
      await attachTag(request, a.id, solo.id);

      await page.goto("/");
      await expect(page.getByRole("article")).toHaveCount(4);

      await checkbox(page, "タグ動画A").click();
      await checkbox(page, "タグ動画B").click();
      await checkbox(page, "タグ動画D").click();
      await expect(page.getByText("3 videos selected")).toBeVisible();

      await page.getByRole("button", { name: "Add tag" }).click();
      const addInput = page.getByRole("combobox", { name: "Add tag" });
      await addInput.fill("e2e全部タグ");
      await page.getByRole("option", { name: /^Create/ }).click();
      await expect(page.getByText('Added "e2e全部タグ" to 3 videos')).toBeVisible();

      // 3本すべてのカードに付く（受け入れ条件3）。見える「タグ」の一覧の中だけを
      // 見る（オーバーフロー計測用の隠れた複製は数えない）。
      for (const v of [a, b, d]) {
        await expect(
          page
            .locator(`[data-video-id="${String(v.id)}"]`)
            .getByRole("list", { name: "Tags", exact: true })
            .getByText("e2e全部タグ"),
        ).toBeVisible();
      }
      // 再生画面にも表示される（受け入れ条件3）。
      await page.goto(`/videos/${String(b.id)}`);
      await expect(chip(page, "e2e全部タグ")).toBeVisible();
      await page.goto("/");

      // 「タグを外す」を開くと、一部にしか付いていない e2e一部タグ が分かる。
      await checkbox(page, "タグ動画A").click();
      await checkbox(page, "タグ動画B").click();
      await checkbox(page, "タグ動画D").click();
      await page.getByRole("button", { name: "Remove tag" }).click();
      await expect(
        page.getByRole("option", { name: "e2e一部タグ, only some videos, 1 of 3" }),
      ).toBeVisible();
      await expect(page.getByRole("option", { name: /^e2e全部タグ/ })).toBeVisible();
    });

    test("4: すべて選択で全件を選んでタグを付けると、選択バーの件数どおり全動画に付く", async ({
      page,
      request,
    }) => {
      await page.goto("/");
      await expect(page.getByRole("article")).toHaveCount(4);

      await checkbox(page, "タグ動画A").click();
      await expect(page.getByText("1 video selected")).toBeVisible();

      await page.getByRole("button", { name: "Select all" }).click();
      await expect(page.getByText("4 videos selected")).toBeVisible();

      await page.getByRole("button", { name: "Add tag" }).click();
      await page.getByRole("combobox", { name: "Add tag" }).fill("e2eすべて選択タグ");
      await page.getByRole("option", { name: /^Create/ }).click();
      await expect(page.getByText('Added "e2eすべて選択タグ" to 4 videos')).toBeVisible();

      // 選択バーに出ていた件数（4件、ライブラリの全件）と同じ本数に付いたことを、
      // タグの本数（GET /api/tags の videoCount）で確かめる（受け入れ条件4）。
      const tagsResponse = await request.get("/api/tags");
      const tags = ((await tagsResponse.json()) as { items: Tag[] }).items;
      const created = tags.find((t) => t.name === "e2eすべて選択タグ");
      expect(created?.videoCount).toBe(4);
    });
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
      await expect(async () => {
        if ((await row.count()) > 0) return;
        await page.evaluate(() => {
          const before = window.scrollY;
          window.scrollBy(0, 400);
          if (window.scrollY === before) window.scrollTo(0, 0);
        });
        await expect(row).toBeAttached({ timeout: 300 });
      }).toPass({ timeout: 15_000, intervals: [0] });
      await row.scrollIntoViewIfNeeded();
      return row;
    }

    test("9: 新しいタグを作ると本数0で一覧に並び、再生画面の候補にも出る", async ({
      page,
    }) => {
      await page.goto("/tags");
      // 操作の行の「新しいタグ」と、タグが無い状態の primary「新しいタグ」は同じ
      // 読み上げ名を持つ（ui-design.md「Tag management page」）。ここは常に出て
      // いる操作の行の1つ目を押す。
      await page.getByRole("button", { name: "New tag" }).first().click();
      const input = page.getByRole("textbox", { name: "New tag name" });
      await input.fill("e2e管理新規");
      await page.keyboard.press("Enter");

      const row = tagRowByName(page, "e2e管理新規");
      await expect(row).toBeVisible();
      await expect(row).toContainText("0 videos");

      const a = video("タグ動画A");
      await page.goto(`/videos/${String(a.id)}`);
      await addInput(page).click();
      await expect(page.getByRole("option", { name: /e2e管理新規/ })).toBeVisible();
    });

    test("10: 改名すると付いた動画のカードに新しい名前が出て、既存の名前と重なる改名は理由を示す", async ({
      page,
      request,
    }) => {
      const created = await createTag(request, "e2e管理改名前");
      const a = video("タグ動画A");
      await clearVideoTags(request, a.id);
      await attachTag(request, a.id, created.id);

      await page.goto("/tags");
      await revealTagRow(page, "e2e管理改名前");
      await tagRowByName(page, "e2e管理改名前")
        .getByRole("button", { name: "Rename" })
        .click();
      const input = page.getByRole("textbox", { name: 'New name for "e2e管理改名前"' });
      await input.fill("e2e管理改名後");
      await page.keyboard.press("Enter");
      await expect(tagRowByName(page, "e2e管理改名後")).toBeVisible();

      // 付いていたカードにも新しい名前が出る。
      await page.goto("/");
      await expect(
        page
          .locator(`[data-video-id="${String(a.id)}"]`)
          .getByRole("button", { name: "Filter by e2e管理改名後" }),
      ).toBeVisible();

      // 再生画面にも新しい名前が出る。
      await page.goto(`/videos/${String(a.id)}`);
      await expect(page.locator('[title="e2e管理改名後"]')).toBeVisible();
      await expect(page.locator('[title="e2e管理改名前"]')).toHaveCount(0);

      // 既存のタグ名と重なる改名は拒否され、理由が画面に出る。
      await createTag(request, "e2e管理既存名");
      await page.goto("/tags");
      await revealTagRow(page, "e2e管理改名後");
      await tagRowByName(page, "e2e管理改名後")
        .getByRole("button", { name: "Rename" })
        .click();
      const renameInput = page.getByRole("textbox", {
        name: 'New name for "e2e管理改名後"',
      });
      await renameInput.fill("e2e管理既存名");
      await page.keyboard.press("Enter");
      await expect(
        page.getByText('A tag named "e2e管理既存名" already exists.'),
      ).toBeVisible();
      // 入力は残る。
      await expect(renameInput).toHaveValue("e2e管理既存名");
    });

    test("11: 削除の確認で外れる本数が示され、確定するとどの動画からもタグが消える", async ({
      page,
      request,
    }) => {
      const created = await createTag(request, "e2e管理削除対象");
      const a = video("タグ動画A");
      await clearVideoTags(request, a.id);
      await attachTag(request, a.id, created.id);

      await page.goto("/tags");
      await revealTagRow(page, "e2e管理削除対象");
      const row = tagRowByName(page, "e2e管理削除対象");
      await row.getByRole("button", { name: "More actions" }).click();
      await page.getByRole("menuitem", { name: "Delete…" }).click();

      const dialog = page.getByRole("alertdialog", { name: 'Delete "e2e管理削除対象"' });
      await expect(
        dialog.getByText("This tag will be removed from 1 video. This can't be undone."),
      ).toBeVisible();
      await dialog.getByRole("button", { name: "Delete" }).click();

      await expect(tagRowByName(page, "e2e管理削除対象")).toHaveCount(0);
      await expect(page.getByText('Deleted "e2e管理削除対象"')).toBeVisible();

      await page.goto(`/videos/${String(a.id)}`);
      await expect(page.locator('[title="e2e管理削除対象"]')).toHaveCount(0);
    });

    test("14: どの動画からも外したタグは本数0で管理画面に残り、候補にも出続ける", async ({
      page,
      request,
    }) => {
      const created = await createTag(request, "e2e管理残留");
      const a = video("タグ動画A");
      await clearVideoTags(request, a.id);
      await attachTag(request, a.id, created.id);
      const detached = await request.post("/api/video-tags", {
        headers: mutationHeaders,
        data: { videoIds: [a.id], action: "remove", tag: { id: created.id } },
      });
      expect(detached.ok()).toBe(true);

      await page.goto("/tags");
      await revealTagRow(page, "e2e管理残留");
      await expect(tagRowByName(page, "e2e管理残留")).toContainText("0 videos");

      await page.goto(`/videos/${String(a.id)}`);
      await addInput(page).click();
      await expect(page.getByRole("option", { name: /e2e管理残留/ })).toBeVisible();
    });

    test("16: 管理画面でタグを選ぶと、そのタグだけで絞り込んだライブラリ一覧が開く", async ({
      page,
      request,
    }) => {
      const created = await createTag(request, "e2e管理移動先");
      const a = video("タグ動画A");
      const b = video("タグ動画B");
      await clearVideoTags(request, a.id);
      await clearVideoTags(request, b.id);
      await attachTag(request, a.id, created.id);

      await page.goto("/tags");
      await revealTagRow(page, "e2e管理移動先");
      await tagRowByName(page, "e2e管理移動先").click();
      await expect(page).toHaveURL(
        new RegExp(`^http://127\\.0\\.0\\.1:15173/\\?tag=${String(created.id)}$`),
      );

      // URL だけでなく、実際にそのタグ1つで絞り込んだ一覧になっている
      // （タグの付いた A だけが残り、絞り込み中のタグの行にも出る）。
      await expect(page.getByRole("article")).toHaveCount(1);
      await expect(page.getByRole("link", { name: "タグ動画A" })).toBeVisible();
      await expect(page.getByRole("link", { name: "タグ動画B" })).toHaveCount(0);
      await expect(
        page
          .getByRole("list", { name: "Tags in the filter" })
          .getByRole("button", { name: "Remove the filter for e2e管理移動先" }),
      ).toBeVisible();
    });

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
      }
      await page.setViewportSize({ width: 1280, height: 800 });
    });

    test("18: 検索は名前とシノニムに大文字小文字を区別せず当たり、消すと全件に戻る", async ({
      page,
      request,
    }) => {
      const anime = await createTag(request, "e2eXyz9Anime管理");
      await addSynonym(request, anime.id, "e2eXyz9アニメ管理");
      await createTag(request, "e2eXyz9Drama管理");

      await page.goto("/tags");
      const search = page.getByRole("searchbox", { name: "Search tags" });

      // 名前「e2eXyz9Anime管理」自体にはカタカナが無い。シノニム
      // 「e2eXyz9アニメ管理」が「アニ」を含むので、シノニムでの一致として当たる。
      await search.fill("e2eXyz9アニ");
      await expect(tagRowByName(page, "e2eXyz9Anime管理")).toBeVisible();
      await expect(tagRowByName(page, "e2eXyz9Drama管理")).toHaveCount(0);

      // 名前「Anime」に「ani」が含まれ、大文字小文字を区別しない。
      await search.fill("e2eXyz9ani");
      await expect(tagRowByName(page, "e2eXyz9Anime管理")).toBeVisible();
      await expect(tagRowByName(page, "e2eXyz9Drama管理")).toHaveCount(0);

      await search.fill("");
      await revealTagRow(page, "e2eXyz9Anime管理");
      await expect(tagRowByName(page, "e2eXyz9Anime管理")).toBeVisible();
      await revealTagRow(page, "e2eXyz9Drama管理");
      await expect(tagRowByName(page, "e2eXyz9Drama管理")).toBeVisible();
    });

    test("キーボードだけで検索の入力に届き、一致が無いときはタグが無い状態と別表示になる", async ({
      page,
      request,
    }) => {
      await createTag(request, "e2e管理キーボード対象");
      await page.goto("/tags");
      // 画面は必要になったときに読み込む（web/src/app/deferredRoute.tsx）。利用者と
      // 同じく、画面が出てからキーを押す。
      await expect(page.getByRole("heading", { level: 1, name: "Tags" })).toBeVisible();

      // `/` で検索の入力へ移る（ui-design.md の操作の確認 手順4、ライブラリの検索欄と同じ）。
      await page.keyboard.press("/");
      const search = page.getByRole("searchbox", { name: "Search tags" });
      await expect(search).toBeFocused();

      await search.fill("e2e管理存在しない語");
      await expect(page.getByText('No tags match "e2e管理存在しない語"')).toBeVisible();
      await expect(page.getByText("No tags yet")).toHaveCount(0);

      await page.getByRole("button", { name: "Show all tags" }).click();
      await expect(search).toBeFocused();
      await expect(search).toHaveValue("");
      await revealTagRow(page, "e2e管理キーボード対象");
      await expect(tagRowByName(page, "e2e管理キーボード対象")).toBeVisible();
    });

    test("空・長すぎる名前と既存のタグの名前は、作成の行に英語の理由が出て作られない", async ({
      page,
      request,
    }) => {
      await createTag(request, "e2e管理理由既存");
      await page.goto("/tags");
      await page.getByRole("button", { name: "New tag" }).first().click();
      const input = page.getByRole("textbox", { name: "New tag name" });

      await page.getByRole("button", { name: "Create", exact: true }).click();
      await expect(page.getByText("Enter a name")).toBeVisible();

      await input.fill("あ".repeat(101));
      await expect(
        page.getByText("Use 100 characters or fewer (currently 101)"),
      ).toBeVisible();

      await input.fill("e2e管理理由既存");
      await page.keyboard.press("Enter");
      await expect(
        page.getByText('A tag named "e2e管理理由既存" already exists.'),
      ).toBeVisible();
      await expect(input).toHaveValue("e2e管理理由既存");
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

    test("削除確認の窓でEscを押すと何も変わらない", async ({ page, request }) => {
      await createTag(request, "e2e管理Esc確認");
      await page.goto("/tags");
      await revealTagRow(page, "e2e管理Esc確認");
      const row = tagRowByName(page, "e2e管理Esc確認");
      await row.getByRole("button", { name: "More actions" }).click();
      await page.getByRole("menuitem", { name: "Delete…" }).click();
      const dialog = page.getByRole("alertdialog", { name: 'Delete "e2e管理Esc確認"' });
      await expect(dialog).toBeVisible();

      await page.keyboard.press("Escape");

      await expect(page.getByRole("alertdialog")).toHaveCount(0);
      await expect(tagRowByName(page, "e2e管理Esc確認")).toBeVisible();
    });

    test("別のタブで消したタグを改名しようとすると、もう無いことが伝わり一覧が取り直される", async ({
      page,
      request,
    }) => {
      const created = await createTag(request, "e2e管理消滅");
      await page.goto("/tags");
      await revealTagRow(page, "e2e管理消滅");
      await tagRowByName(page, "e2e管理消滅")
        .getByRole("button", { name: "Rename" })
        .click();
      const input = page.getByRole("textbox", { name: 'New name for "e2e管理消滅"' });

      const removed = await request.delete(`/api/tags/${String(created.id)}`, {
        headers: mutationHeaders,
      });
      expect(removed.status()).toBe(204);

      await input.fill("e2e管理消滅後");
      await page.keyboard.press("Enter");

      await expect(
        page.getByText("This tag no longer exists, so the list was reloaded"),
      ).toBeVisible();
      await expect(tagRowByName(page, "e2e管理消滅")).toHaveCount(0);
      await expect(tagRowByName(page, "e2e管理消滅後")).toHaveCount(0);
    });

    test("統合と統合先の選択は、その他の操作のメニューの中にあり、削除より先に並ぶ（作成・改名より目立たない）", async ({
      page,
      request,
    }) => {
      await createTag(request, "e2e管理統合メニュー");
      await page.goto("/tags");
      await revealTagRow(page, "e2e管理統合メニュー");
      const row = tagRowByName(page, "e2e管理統合メニュー");

      // 行に直接出るのは「改名」と「シノニム」だけで、統合は行に出ない。
      await expect(
        row.getByRole("button", { name: "Merge into another tag…" }),
      ).toHaveCount(0);

      await row.getByRole("button", { name: "More actions" }).click();
      const items = page.getByRole("menuitem");
      await expect(items).toHaveText(["Merge into another tag…", "Delete…"]);
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

    test("統合の確認は選ぶまでdisabled、統合先の候補は統合元を除き作成の行を持たない", async ({
      page,
      request,
    }) => {
      await createTag(request, "e2e管理統合候補元");
      await createTag(request, "e2e管理統合候補先");
      await page.goto("/tags");
      await revealTagRow(page, "e2e管理統合候補元");
      const row = tagRowByName(page, "e2e管理統合候補元");
      await row.getByRole("button", { name: "More actions" }).click();
      await page.getByRole("menuitem", { name: "Merge into another tag…" }).click();

      const dialog = page.getByRole("dialog", { name: 'Merge "e2e管理統合候補元"' });
      const mergeButton = dialog.getByRole("button", { name: "Merge" });
      await expect(mergeButton).toBeDisabled();

      const combo = dialog.getByRole("combobox", { name: "Tag to merge into" });
      await combo.click();
      await expect(page.getByRole("option", { name: /e2e管理統合候補元/ })).toHaveCount(
        0,
      );

      await combo.fill("e2e管理統合候補先には無い語zzz");
      await expect(page.getByText(/^Create "/)).toHaveCount(0);
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

    test("17: 既存のタグの名前をシノニムとして登録しようとすると統合の確認が出て、承諾すると統合され、取り消すと何も変わらない（受け入れ条件17）", async ({
      page,
      request,
    }) => {
      await createTag(request, "e2eXyz17Anime");
      const lower = await createTag(request, "e2eXyz17anime");
      const a = video("タグ動画A");
      const b = video("タグ動画B");
      await clearVideoTags(request, a.id);
      await clearVideoTags(request, b.id);
      await attachTag(request, a.id, lower.id);
      await attachTag(request, b.id, lower.id);

      await page.goto("/tags");
      await revealTagRow(page, "e2eXyz17Anime");
      await tagRowByName(page, "e2eXyz17Anime")
        .getByRole("button", { name: "Synonyms" })
        .click();
      const dialog = page.getByRole("dialog", { name: 'Synonyms of "e2eXyz17Anime"' });
      const input = dialog.getByRole("textbox", { name: "Add synonym" });
      await input.fill("e2eXyz17anime");
      await page.keyboard.press("Enter");

      await expect(
        dialog.getByText(
          '"e2eXyz17anime" is a tag on 2 videos. Merging it into "e2eXyz17Anime" adds ' +
            '"e2eXyz17Anime" to those videos, and "e2eXyz17anime" becomes a synonym of ' +
            '"e2eXyz17Anime". "e2eXyz17anime" leaves the tag list.',
        ),
      ).toBeVisible();

      // 取り消す（戻る）と何も変わらない。
      await dialog.getByRole("button", { name: "Back" }).click();
      await expect(dialog.getByText(/is a tag on/)).toHaveCount(0);
      await expect(input).toHaveValue("e2eXyz17anime");
      await dialog.getByRole("button", { name: "Close" }).click();
      await revealTagRow(page, "e2eXyz17anime");
      await expect(tagRowByName(page, "e2eXyz17anime")).toBeVisible();
      await revealTagRow(page, "e2eXyz17Anime");

      // もう一度、今度は承諾する。
      await tagRowByName(page, "e2eXyz17Anime")
        .getByRole("button", { name: "Synonyms" })
        .click();
      const dialog2 = page.getByRole("dialog", { name: 'Synonyms of "e2eXyz17Anime"' });
      await dialog2.getByRole("textbox", { name: "Add synonym" }).fill("e2eXyz17anime");
      await page.keyboard.press("Enter");
      await dialog2.getByRole("button", { name: "Merge" }).click();

      // getByText は大文字小文字を区別しない部分一致なので、窓の見出し
      // 「「e2eXyz17Anime」のシノニム」と取り違えないよう exact にする。
      await expect(dialog2.getByText("e2eXyz17anime", { exact: true })).toBeVisible();
      await dialog2.getByRole("button", { name: "Close" }).click();
      await expect(tagRowByName(page, "e2eXyz17anime")).toHaveCount(0);
      await revealTagRow(page, "e2eXyz17Anime");
      await expect(tagRowByName(page, "e2eXyz17Anime")).toContainText(
        "Synonyms: e2eXyz17anime",
      );

      // 承諾したあと、再生画面で `e2eXyz17anime` と入力して確定すると
      // `e2eXyz17Anime` が付く（受け入れ条件13と同じ仕組み）。
      const c = video("タグ動画C");
      await clearVideoTags(request, c.id);
      await page.goto(`/videos/${String(c.id)}`);
      const addTagInput = addInput(page);
      await addTagInput.click();
      await addTagInput.fill("e2eXyz17anime");
      await page.keyboard.press("Enter");
      await expect(chip(page, "e2eXyz17Anime")).toBeVisible();
    });

    test("別のタグのシノニムと同じ名前の登録で、そのタグの名前が画面に出る（シノニム名の衝突）", async ({
      page,
      request,
    }) => {
      const anime = await createTag(request, "e2e管理衝突Anime");
      await addSynonym(request, anime.id, "e2e管理衝突アニメ");
      await createTag(request, "e2e管理衝突Drama");

      await page.goto("/tags");
      await revealTagRow(page, "e2e管理衝突Drama");
      await tagRowByName(page, "e2e管理衝突Drama")
        .getByRole("button", { name: "Synonyms" })
        .click();
      const dialog = page.getByRole("dialog", { name: 'Synonyms of "e2e管理衝突Drama"' });
      const input = dialog.getByRole("textbox", { name: "Add synonym" });
      await input.fill("e2e管理衝突アニメ");
      await page.keyboard.press("Enter");

      await expect(
        dialog.getByText(
          '"e2e管理衝突アニメ" is already a synonym of the tag "e2e管理衝突Anime".',
        ),
      ).toBeVisible();
    });

    test("シノニムの×は確認なしにその場で解除できる", async ({ page, request }) => {
      const anime = await createTag(request, "e2e管理解除Anime");
      await addSynonym(request, anime.id, "e2e管理解除アニメ");

      await page.goto("/tags");
      await revealTagRow(page, "e2e管理解除Anime");
      await tagRowByName(page, "e2e管理解除Anime")
        .getByRole("button", { name: "Synonyms" })
        .click();
      const dialog = page.getByRole("dialog", { name: 'Synonyms of "e2e管理解除Anime"' });
      await dialog
        .getByRole("button", { name: 'Remove the synonym "e2e管理解除アニメ"' })
        .click();
      await expect(dialog.getByText("e2e管理解除アニメ")).toHaveCount(0);
      await dialog.getByRole("button", { name: "Close" }).click();

      await expect(tagRowByName(page, "e2e管理解除Anime")).not.toContainText("Synonyms:");
    });
  });
});
