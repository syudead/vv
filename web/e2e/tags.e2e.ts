import { expect, test, type APIRequestContext, type Page } from "@playwright/test";

// 再生画面のタグ（issue 268、親 Issue #193 の受け入れ条件 1・2・6・13）と、
// ライブラリのカードのタグ・タグでの絞り込み（issue 269、受け入れ条件 5・7・8・19・20）、
// 選択バーの一括操作・すべて選択（issue 270、受け入れ条件 3・4）を実ブラウザに通す。
// 動画は run-e2e.mjs が generateTagsFixtures で作る 3 本
// （タグ動画A・タグ動画B・タグ動画C）。

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
  return page.getByRole("combobox", { name: "タグを追加" });
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
    expect(page.items).toHaveLength(3);
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
      name: "e2e外すタグをこの動画から外す",
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
    await expect(page.getByText("名前を入力してください")).toBeVisible();

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
    await expect(page.getByText("改行やタブは使えません")).toBeVisible();
    await expect(input).toHaveValue("");

    // 101 文字は確定できない。
    await input.fill("あ".repeat(101));
    await expect(
      page.getByText("100 文字以内にしてください（今 101 文字）"),
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
        .getByRole("list", { name: "絞り込み中のタグ" })
        .getByRole("button", { name: `${name}の絞り込みを外す` });
    }

    /**
     * pressCardTag は、カードのタグを押して絞り込む。ほかのテストで付けたタグが
     * 積み重なって1行に収まらないことがあるので、直に見えなければ「+N」を開いて
     * そこから押す（ui-design.md「Overflow」: 「+N」のチップを押しても同じく絞り込む）。
     */
    async function pressCardTag(page: Page, videoId: number, name: string) {
      const card = page.locator(`[data-video-id="${String(videoId)}"]`);
      const direct = card.getByRole("button", { name: `${name}で絞り込む` });
      if ((await direct.count()) > 0) {
        await direct.click();
        return;
      }
      await card.getByRole("button", { name: /^ほかのタグ \d+ 個を表示$/ }).click();
      await page
        .getByRole("dialog")
        .getByRole("button", { name: `${name}で絞り込む` })
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
      await expect(card.getByRole("list", { name: "タグ" })).toBeVisible();
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
        .getByRole("button", { name: /^ほかのタグ \d+ 個を表示$/ });
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
      // ライブラリには3本ある（タグ動画A・B・C）。
      await expect(page.getByRole("article")).toHaveCount(3);

      await pressCardTag(page, a.id, "e2e旅行");
      await expect(page).toHaveURL(new RegExp(`tag=${String(tag.id)}(&|$)`));
      // タグの付いた両方が残り、検索欄は変わらない。
      await expect(page.getByRole("article")).toHaveCount(2);
      await expect(activeTagChip(page, "e2e旅行")).toBeVisible();

      // 一覧の上のチップを1回押すと外れ、押す前の一覧に戻る。
      await activeTagChip(page, "e2e旅行").click();
      await expect(page).not.toHaveURL(/tag=/);
      await expect(page.getByRole("article")).toHaveCount(2);

      // 再生画面でタグを押すと、そのタグ1つで絞り込んだライブラリ一覧が開く。
      await page.goto(`/videos/${String(a.id)}`);
      await page.getByRole("link", { name: "e2e旅行で絞り込む" }).click();
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
      const b = video("タグ動画B");
      await attachTag(request, a.id, searchable.id);

      await page.goto("/");
      const box = page.getByRole("searchbox", { name: "動画を検索" });

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
    });
  });

  test.describe("選択バーの一括操作・すべて選択（issue 270）", () => {
    function checkbox(page: Page, title: string) {
      return page.getByRole("checkbox", { name: `「${title}」を選択` });
    }

    test("3: 3本を選び選択バーからタグを付けると全部に付く。外す候補で一部にしか付いていないタグが分かる", async ({
      page,
      request,
    }) => {
      const a = video("タグ動画A");
      const b = video("タグ動画B");
      const c = video("タグ動画C");
      await clearVideoTags(request, a.id);
      await clearVideoTags(request, b.id);
      await clearVideoTags(request, c.id);
      const solo = await createTag(request, "e2e一部タグ");
      await attachTag(request, a.id, solo.id);

      await page.goto("/");
      await expect(page.getByRole("article")).toHaveCount(3);

      await checkbox(page, "タグ動画A").click();
      await checkbox(page, "タグ動画B").click();
      await checkbox(page, "タグ動画C").click();
      await expect(page.getByText("3 件を選択中")).toBeVisible();

      await page.getByRole("button", { name: "タグを付ける" }).click();
      const addInput = page.getByRole("combobox", { name: "タグを付ける" });
      await addInput.fill("e2e全部タグ");
      await page.getByRole("option", { name: /を作成/ }).click();
      await expect(page.getByText("3 件に「e2e全部タグ」を付けました")).toBeVisible();

      // 3本すべてのカードに付く（受け入れ条件3）。見える「タグ」の一覧の中だけを
      // 見る（オーバーフロー計測用の隠れた複製は数えない）。
      for (const v of [a, b, c]) {
        await expect(
          page
            .locator(`[data-video-id="${String(v.id)}"]`)
            .getByRole("list", { name: "タグ" })
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
      await checkbox(page, "タグ動画C").click();
      await page.getByRole("button", { name: "タグを外す" }).click();
      await expect(
        page.getByRole("option", { name: "e2e一部タグ、一部の動画だけ、3 件中 1 件" }),
      ).toBeVisible();
      await expect(page.getByRole("option", { name: /^e2e全部タグ/ })).toBeVisible();
    });

    test("4: すべて選択で全件を選んでタグを付けると、選択バーの件数どおり全動画に付く", async ({
      page,
      request,
    }) => {
      await page.goto("/");
      await expect(page.getByRole("article")).toHaveCount(3);

      await checkbox(page, "タグ動画A").click();
      await expect(page.getByText("1 件を選択中")).toBeVisible();

      await page.getByRole("button", { name: "すべて選択" }).click();
      await expect(page.getByText("3 件を選択中")).toBeVisible();

      await page.getByRole("button", { name: "タグを付ける" }).click();
      await page
        .getByRole("combobox", { name: "タグを付ける" })
        .fill("e2eすべて選択タグ");
      await page.getByRole("option", { name: /を作成/ }).click();
      await expect(
        page.getByText("3 件に「e2eすべて選択タグ」を付けました"),
      ).toBeVisible();

      // 選択バーに出ていた件数（3件、ライブラリの全件）と同じ本数に付いたことを、
      // タグの本数（GET /api/tags の videoCount）で確かめる（受け入れ条件4）。
      const tagsResponse = await request.get("/api/tags");
      const tags = ((await tagsResponse.json()) as { items: Tag[] }).items;
      const created = tags.find((t) => t.name === "e2eすべて選択タグ");
      expect(created?.videoCount).toBe(3);
    });
  });
});
