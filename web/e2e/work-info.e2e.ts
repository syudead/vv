import { expect, test, type APIRequestContext, type Page } from "@playwright/test";

// 動画の作品情報の長い説明（親 Issue #967 の Edge Cases「長い説明で崩れない」、子 #1030）を
// 実ブラウザに通す。畳むかどうかは描画した高さで決まり、横スクロールが出ないことも実際の
// レイアウトでしか確かめられない。編集・保存・外部 API・項目の出し方は Go のストアとハンドラー、
// Vitest のページとコンポーネントのテストが持つ（docs/design-docs/web-testing.md の Test levels）。
// 動画は run-e2e.mjs が generateWorkInfoFixtures で作る作品動画A〜D の 4 本。人物はフォルダを
// 外しても残るので、名前の末尾に実行ごとの印 run を付け、CI の再試行（同じサーバーと DB で
// ワーカーを作り直す）が前の試行の同名の人物に当たらないようにする。作品情報は動画の中身に
// 付いて残り、再登録と再スキャンで戻ってくるので、beforeAll で消す。

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

/** run は試行ごとに変わる印。再試行はワーカーを作り直すので、このモジュールも読み直される。 */
const run = Date.now().toString(36);

/** 入れる値。名前は他の e2e の人物・団体に当たらないものにする。 */
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

/** clearDetails は前の試行が残した表示名と作品情報を消し、ファイル名の題名に戻った動画を返す。 */
async function clearDetails(request: APIRequestContext, id: number): Promise<Video> {
  const response = await request.put(`/api/videos/${String(id)}/details`, {
    headers: mutationHeaders,
    data: { displayName: "", workInfo: {} },
  });
  expect(response.ok()).toBe(true);
  return (await response.json()) as Video;
}

/** infoArea は動画ページの情報欄（ファイルの行・作品情報・技術の行を縦に並べる枠）である。 */
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

    // 開いても横には広がらず、閉じると畳んだ状態に戻る。
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
