import { expect, test, type APIRequestContext, type Page } from "@playwright/test";

// 動画の作品情報（親 Issue #967 の受け入れ条件 1〜8 と Edge Cases の「長い説明で崩れない」、
// 子 #1030）を実ブラウザに通す。動画は run-e2e.mjs が generateWorkInfoFixtures で作る
// 作品動画A〜D の 4 本。テストは順に状態を積む: A に編集画面で値を入れ、B・C には画面の API で
// 絞り込みと統合のための値を置き、D には撮影日と撮影者だけを入れる。人物と団体はサーバーの
// 中で他の e2e と共有するので、名前は「作品e2e」で始めて他と重ならないようにする。
// 人物・団体・別名はフォルダを外しても残るので、名前の末尾に実行ごとの印 run を付け、
// CI の再試行（同じサーバーと DB でワーカーを作り直す）が前の試行の同名の候補に当たらないようにする。
// 表示名と作品情報は動画の中身に付いて残り、再登録と再スキャンで戻ってくるので、beforeAll で消す。
// テスト名の番号は受け入れ条件の番号で、値を消す 2 は値を使う他のテストの後に置く。

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

interface ExternalVideo {
  workInfo: {
    code: string | null;
    dates: { kind: string; date: string }[];
    people: { role: string; person: { id: number; name: string } }[];
  };
}

const origin = "http://127.0.0.1:15173";
const mutationHeaders = { Origin: origin, "Content-Type": "application/json" };

/** run は試行ごとに変わる印。再試行はワーカーを作り直すので、このモジュールも読み直される。 */
const run = Date.now().toString(36);

/** 入れる値。品番・名前・別名は検索で他の動画に当たらないものにする。 */
const displayName = "作品e2e表示名";
const code = "WIE-047";
const released = "2024-06-01";
const haruka = `作品e2e春香${run}`;
const harukaAlias = `HarukaE2E${run}`;
const natsumi = `作品e2e夏美${run}`;
const akiko = `作品e2e秋子${run}`;
const maker = `作品e2eメーカー${run}`;
const photographer = `作品e2e撮影者${run}`;
const externalCode = "EXT-047";

let folder: MediaFolder | undefined;
const videos = new Map<string, Video>();
const people = new Map<string, Credit>();
let makerId: number | undefined;
/** 作品情報を入れる前の、A の情報欄の行の数。消した後に同じになることを確かめる。 */
let rowsBefore: number | undefined;

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
  data: { name: string; aliases?: { name: string }[] },
): Promise<Credit> {
  const response = await request.post(`/api/${kind}`, { headers: mutationHeaders, data });
  expect(response.status()).toBe(201);
  return (await response.json()) as Credit;
}

/** findPerson は名前の完全一致で人物を引く（編集画面で作った人物の id を知るため）。 */
async function findPerson(request: APIRequestContext, name: string): Promise<Credit> {
  const response = await request.get(`/api/people?q=${encodeURIComponent(name)}`);
  expect(response.ok()).toBe(true);
  const { items } = (await response.json()) as { items: Credit[] };
  const found = items.find((item) => item.name === name);
  if (found === undefined) throw new Error(`no person named ${name}`);
  return found;
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

async function issueToken(request: APIRequestContext): Promise<string> {
  const issued = await request.post("/api/api-tokens", {
    headers: mutationHeaders,
    data: { name: "e2e作品情報" },
  });
  expect(issued.status()).toBe(201);
  return ((await issued.json()) as { secret: string }).secret;
}

/** item は作品情報の行の 1 項目（ラベルと値）に合う形である。 */
function item(label: string, value: string): RegExp {
  const escape = (text: string) => text.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
  return new RegExp(`^${escape(label)}\\s*${escape(value)}$`);
}

function workDetails(page: Page) {
  return page.getByRole("list", { name: "Work details" });
}

/** infoArea は動画ページの情報欄（ファイルの行・作品情報・技術の行を縦に並べる枠）である。 */
function infoArea(page: Page) {
  return page.getByRole("list", { name: "File details" }).locator("xpath=../..");
}

async function openVideo(page: Page, id: number, title: string) {
  await page.goto(`/videos/${String(id)}`);
  await expect(page.getByRole("heading", { level: 1 })).toHaveText(title);
}

async function openEditor(page: Page) {
  await page.getByRole("link", { name: "Edit details" }).click();
  await expect(
    page.getByRole("heading", { level: 1, name: "Edit details" }),
  ).toBeVisible();
}

/** pickExisting は役割の欄に名前を打ち、作る行ではなく既存の候補を選ぶ。 */
async function pickExisting(page: Page, picker: string, name: string) {
  await page.getByRole("combobox", { name: picker }).fill(name);
  await page
    .getByRole("option")
    .filter({ hasText: name })
    .filter({ hasNotText: "Create" })
    .click();
  await expect(page.getByRole("button", { name: `Remove ${name}` })).toBeVisible();
}

/** pickNew は役割の欄に名前を打ち、「Create "…"」でその場で作る。 */
async function pickNew(page: Page, picker: string, name: string) {
  await page.getByRole("combobox", { name: picker }).fill(name);
  await page.getByRole("option", { name: `Create "${name}"` }).click();
  await expect(page.getByRole("button", { name: `Remove ${name}` })).toBeVisible();
}

async function saveEditor(page: Page, title: string) {
  await page.getByRole("button", { name: "Save", exact: true }).click();
  await expect(page).toHaveURL(/\/videos\/\d+$/);
  await expect(page.getByRole("heading", { level: 1 })).toHaveText(title);
}

function cardTitles(page: Page): Promise<string[]> {
  return page.locator("[data-video-id] h3").allTextContents();
}

async function expectLibrary(page: Page, titles: string[]) {
  await expect
    .poll(async () => (await cardTitles(page)).sort())
    .toEqual([...titles].sort());
}

async function searchLibrary(page: Page, query: string, titles: string[]) {
  await page.goto(`/library?q=${encodeURIComponent(query)}`);
  await expectLibrary(page, titles);
}

/** expectFilledA は、編集画面で A に入れた値が情報欄にそのまま出ていることを確かめる。 */
async function expectFilledA(page: Page) {
  await expect(page.getByRole("heading", { level: 1 })).toHaveText(displayName);
  await expect(workDetails(page).getByRole("listitem")).toHaveText([
    item("Code", code),
    item("Released", released),
    item("Performers", `${haruka}, ${natsumi}`),
    item("Maker", maker),
  ]);
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

    // 編集画面では春香を既存の人物から選び、夏美とメーカーはその場で作る。
    people.set(
      haruka,
      await createCredit(request, "people", {
        name: haruka,
        aliases: [{ name: harukaAlias }],
      }),
    );
    people.set(akiko, await createCredit(request, "people", { name: akiko }));
  });

  test.afterAll(async ({ request }) => {
    if (folder === undefined) return;
    const removed = await request.delete(
      `/api/media-folders/${String(folder.id)}?version=${String(folder.version)}`,
      { headers: mutationHeaders },
    );
    expect(removed.status()).toBe(204);
  });

  test("1: ページから開いた編集画面の 1 回の保存で、題名と情報欄の項目になり、再読み込み後も残る", async ({
    page,
    request,
  }) => {
    const a = video("作品動画A");
    await openVideo(page, a.id, a.title);
    // 何も入っていない動画には作品情報の行も説明も無い。
    await expect(workDetails(page)).toHaveCount(0);
    rowsBefore = await infoArea(page).locator("> *").count();

    await openEditor(page);
    await expect(page.getByRole("textbox", { name: "Name" })).toBeFocused();
    await page.getByRole("textbox", { name: "Name" }).fill(displayName);
    await page.getByRole("textbox", { name: "Code" }).fill(code);
    await page.getByRole("textbox", { name: "Released" }).fill(released);
    await pickExisting(page, "Add a performer", haruka);
    await pickNew(page, "Add a performer", natsumi);
    await pickNew(page, "Add a maker", maker);
    await saveEditor(page, displayName);

    await expectFilledA(page);
    await expect(page).toHaveTitle(`${displayName} · VVMDM`);
    await page.reload();
    await expectFilledA(page);

    people.set(natsumi, await findPerson(request, natsumi));
    const makers = await request.get(`/api/organizations?q=${encodeURIComponent(maker)}`);
    const found = ((await makers.json()) as { items: Credit[] }).items.find(
      (item) => item.name === maker,
    );
    if (found === undefined) throw new Error("the maker was not created");
    makerId = found.id;
  });

  test("3: 品番・出演者の名前・別名のどれで検索しても動画が見つかる", async ({
    page,
  }) => {
    for (const query of [code, natsumi, harukaAlias]) {
      await searchLibrary(page, query, [displayName]);
    }
  });

  test("4: 出演者を押すとその人物の動画だけが並び、メーカーでも同じ", async ({
    page,
    request,
  }) => {
    if (makerId === undefined) throw new Error("the maker is not known");
    const a = video("作品動画A");
    const b = video("作品動画B");
    const c = video("作品動画C");
    await setWorkInfo(request, b.id, {
      people: [{ role: "performer", person: { id: person(haruka).id } }],
    });
    await setWorkInfo(request, c.id, {
      people: [{ role: "performer", person: { id: person(akiko).id } }],
      organizations: [{ role: "maker", organization: { id: makerId } }],
    });

    await openVideo(page, a.id, displayName);
    await workDetails(page)
      .getByRole("link", { name: `Show videos with ${haruka}` })
      .click();
    await expect(page).toHaveURL(
      new RegExp(`/library\\?person=${String(person(haruka).id)}$`),
    );
    await expectLibrary(page, [displayName, b.title]);

    await page.goBack();
    await expect(page.getByRole("heading", { level: 1 })).toHaveText(displayName);
    await workDetails(page)
      .getByRole("link", { name: `Show videos with ${maker}` })
      .click();
    await expect(page).toHaveURL(
      new RegExp(`/library\\?organization=${String(makerId)}$`),
    );
    await expectLibrary(page, [displayName, c.title]);
  });

  test("5: /people で人物 A を人物 B に統合すると、出演者が B になり、A の名前で動画が見つかる", async ({
    page,
  }) => {
    const c = video("作品動画C");
    await page.goto(`/people?q=${encodeURIComponent(akiko)}`);
    const row = page.locator(`[data-credit-id="${String(person(akiko).id)}"]`);
    await expect(row).toBeVisible();
    await row.getByRole("button", { name: "More actions" }).click();
    await page.getByRole("menuitem", { name: "Merge into another person…" }).click();
    const dialog = page.getByRole("dialog", { name: `Merge "${akiko}"` });
    await dialog.getByRole("combobox", { name: "Person to merge into" }).fill(haruka);
    await dialog.getByRole("option").filter({ hasText: haruka }).click();
    await dialog.getByRole("button", { name: "Merge", exact: true }).click();
    await expect(dialog).toBeHidden();
    await expect(page.getByText(`Merged "${akiko}" into "${haruka}"`)).toBeVisible();

    await openVideo(page, c.id, c.title);
    await expect(
      workDetails(page).getByRole("listitem").filter({ hasText: "Performer" }),
    ).toHaveText(item("Performer", haruka));
    // 秋子の名前は春香の別名になるので、秋子の名前で春香の動画がすべて見つかる。
    await searchLibrary(page, akiko, [displayName, video("作品動画B").title, c.title]);
  });

  test("7: 全体のスキャンの後も値が残っている", async ({ page, request }) => {
    test.setTimeout(90_000);
    await runScan(request);
    await openVideo(page, video("作品動画A").id, displayName);
    await expectFilledA(page);
  });

  test("8: 外部 API で書いた値が開いているページに出て、ページで入れた値を外部 API で読める", async ({
    page,
    request,
  }) => {
    if (makerId === undefined) throw new Error("the maker is not known");
    const a = video("作品動画A");
    const c = video("作品動画C");
    const secret = await issueToken(request);
    const authorization = { Authorization: `Bearer ${secret}` };

    await openVideo(page, c.id, c.title);
    await expect(
      workDetails(page).getByRole("listitem").filter({ hasText: "Code" }),
    ).toHaveCount(0);
    const written = await request.post("/api/v1/video-work-info", {
      headers: { ...authorization, "Content-Type": "application/json" },
      data: {
        items: [
          {
            video: { id: c.id },
            workInfo: {
              code: externalCode,
              dates: [{ kind: "published", date: "2025" }],
              people: [{ role: "performer", person: { id: person(natsumi).id } }],
              organizations: [{ role: "maker", organization: { id: makerId } }],
            },
          },
        ],
      },
    });
    expect(written.status()).toBe(200);
    // 再読み込みはしない。video の通知でページが読み直す。
    await expect(workDetails(page).getByRole("listitem")).toHaveText([
      item("Code", externalCode),
      item("Published", "2025"),
      item("Performer", natsumi),
      item("Maker", maker),
    ]);

    const lookup = await request.get(`/api/v1/videos/lookup?id=${String(a.id)}`, {
      headers: authorization,
    });
    expect(lookup.status()).toBe(200);
    const { workInfo } = (await lookup.json()) as ExternalVideo;
    expect(workInfo.code).toBe(code);
    expect(workInfo.dates).toEqual([{ kind: "release", date: released }]);
    expect(workInfo.people.map((entry) => [entry.role, entry.person.name])).toEqual([
      ["performer", haruka],
      ["performer", natsumi],
    ]);
  });

  test("6: 撮影日と撮影者だけを入れると、他の項目は出ない", async ({ page }) => {
    const d = video("作品動画D");
    await openVideo(page, d.id, d.title);
    await openEditor(page);
    await page.getByRole("textbox", { name: "Recorded" }).fill("2023-08");
    await pickNew(page, "Add a photographer", photographer);
    await saveEditor(page, d.title);

    await expect(workDetails(page).getByRole("listitem")).toHaveText([
      item("Recorded", "2023-08"),
      item("Photographer", photographer),
    ]);
    await expect(page.getByRole("button", { name: "Show more" })).toHaveCount(0);
  });

  test("2: 編集画面でそれらを消すと、情報の行が前と同じになる", async ({ page }) => {
    if (rowsBefore === undefined) throw new Error("the rows before are not known");
    const a = video("作品動画A");
    await openVideo(page, a.id, displayName);
    await openEditor(page);
    await page.getByRole("textbox", { name: "Name" }).fill("");
    await page.getByRole("textbox", { name: "Code" }).fill("");
    await page.getByRole("textbox", { name: "Released" }).fill("");
    for (const name of [haruka, natsumi, maker]) {
      await page.getByRole("button", { name: `Remove ${name}` }).click();
    }
    await saveEditor(page, a.title);

    await expect(workDetails(page)).toHaveCount(0);
    await expect(infoArea(page).locator("> *")).toHaveCount(rowsBefore);
    await page.reload();
    await expect(page.getByRole("heading", { level: 1 })).toHaveText(a.title);
    await expect(workDetails(page)).toHaveCount(0);
    await expect(infoArea(page).locator("> *")).toHaveCount(rowsBefore);
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
