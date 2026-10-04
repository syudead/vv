import { writeFile } from "node:fs/promises";

import { expect, test, type Locator, type Page, type Response } from "@playwright/test";

// タグ管理画面（/tags）を規模のデータで測る（specs/036-tag-admin-scale/quickstart.md の
// 「場面と期待」）。`go run ./scripts/tagsbench` から呼ばれ、場面ごとの値を Markdown の表で
// TAGSBENCH_RESULT に書く。
//
// Long Task とフレーム時間はページの中から読む（PerformanceObserver の longtask と
// requestAnimationFrame の間隔）。画面の要素は読み上げ名で引くので、一覧の描き方
// （仮想化など）が変わっても同じ場面を測れる。

const scale = process.env.TAGSBENCH_SCALE ?? "?";
const videos = process.env.TAGSBENCH_VIDEOS ?? "?";
const resultPath = process.env.TAGSBENCH_RESULT;

const account = { username: "tagsbench", password: "tagsbench-password" } as const;

/** 行の名前のリンクの読み上げ名の頭（web/src/i18n/en.ts の tags.row.open）。 */
const rowLinkPrefix = "Open the library filtered by ";
/** 最初の行を開いてから何回測るか。 */
const loadRuns = 3;
/** 待つ上限。3,000 個の規模で遅い変更前の画面でも収まる長さにする。 */
const waitTimeout = 120_000;
/** スクロールで、位置も読み込んだ行も変わらないままこれだけ経ったら末尾を待つのをやめる。 */
const scrollStallTimeout = 60_000;

interface LongTaskEntry {
  start: number;
  duration: number;
}

interface BenchState {
  longTasks: LongTaskEntry[];
  firstRowAt: number | null;
  frames: number[];
  recording: boolean;
}

declare global {
  interface Window {
    __tagsBench?: BenchState;
  }
}

/** installProbes はページの読み込みの前に計測の仕掛けを置く（addInitScript で走る）。 */
function installProbes(prefix: string) {
  const state: BenchState = {
    longTasks: [],
    firstRowAt: null,
    frames: [],
    recording: false,
  };
  window.__tagsBench = state;
  new PerformanceObserver((list) => {
    for (const entry of list.getEntries()) {
      state.longTasks.push({ start: entry.startTime, duration: entry.duration });
    }
  }).observe({ type: "longtask", buffered: true });
  const selector = `a[aria-label^="${prefix}"]`;
  const observer = new MutationObserver(() => {
    if (state.firstRowAt === null && document.querySelector(selector) !== null) {
      state.firstRowAt = performance.now();
      observer.disconnect();
    }
  });
  observer.observe(document, { childList: true, subtree: true });
}

async function now(page: Page): Promise<number> {
  return page.evaluate(() => performance.now());
}

/** settle は描画を 2 回と少し待ち、遅れて届く Long Task の記録を拾えるようにする。 */
async function settle(page: Page) {
  await page.evaluate(
    () =>
      new Promise<void>((resolve) => {
        requestAnimationFrame(() =>
          requestAnimationFrame(() => setTimeout(() => resolve(), 200)),
        );
      }),
  );
}

async function longTasksSince(page: Page, since: number): Promise<LongTaskEntry[]> {
  return page.evaluate(
    (start) =>
      (window.__tagsBench?.longTasks ?? []).filter(
        (task) => task.start + task.duration >= start,
      ),
    since,
  );
}

function maxOf(values: readonly number[]): number {
  return values.reduce((a, b) => Math.max(a, b), 0);
}

function median(values: readonly number[]): number {
  const sorted = [...values].sort((a, b) => a - b);
  return sorted[Math.floor(sorted.length / 2)] ?? 0;
}

function ms(value: number): string {
  return `${Math.round(value).toLocaleString("en-US")} ms`;
}

function longTaskText(tasks: readonly LongTaskEntry[]): string {
  if (tasks.length === 0) return "なし（50 ms 以下）";
  return ms(maxOf(tasks.map((task) => task.duration)));
}

interface Interaction {
  /** 操作から結果が見えるまで（Playwright の往復を含む参考の値）。 */
  elapsed: number;
  longTasks: LongTaskEntry[];
}

/**
 * interact は action（操作と、その結果を待つところまで）の間の Long Task を集める。
 */
async function interact(page: Page, action: () => Promise<void>): Promise<Interaction> {
  const start = await now(page);
  const started = Date.now();
  await action();
  const elapsed = Date.now() - started;
  await settle(page);
  return { elapsed, longTasks: await longTasksSince(page, start) };
}

/** isTagList は、タグの一覧（`GET /api/tags`。統合の窓の候補の検索 `q` を除く）の応答か。 */
function isTagList(response: Response): boolean {
  const url = new URL(response.url());
  return (
    url.pathname === "/api/tags" &&
    response.request().method() === "GET" &&
    !url.searchParams.has("q")
  );
}

interface TagListBody {
  items: unknown[];
  totalAll?: number;
  nextCursor?: string;
}

function bytes(value: number): string {
  return `${value.toLocaleString("en-US")} B`;
}

function rows(page: Page): Locator {
  return page.getByRole("link", { name: new RegExp(`^${rowLinkPrefix}`) });
}

/**
 * filteredCount は絞った件数（「12 of 1,000 tags」。036 の初めの形では続きがあれば
 * 「 · 100 loaded」が添わる）である。
 */
function filteredCount(page: Page): Locator {
  return page
    .getByRole("status")
    .filter({ hasText: / of [\d,]+ tags?( · [\d,]+ loaded)?$/ });
}

/**
 * countLine は件数（「1,000 tags」「12 of 1,000 tags」。036 の初めの形では続きがあれば
 * 「 · 100 loaded」が添わる）である。
 */
function countLine(page: Page): Locator {
  return page
    .getByRole("status")
    .filter({ hasText: /[\d,]+ tags?( · [\d,]+ loaded)?$/ })
    .first();
}

/**
 * listFullyRendered は、画面が最後のページを描いたかを返す。応答が届いたことと、その行が
 * 一覧に描かれたことは別で、描かれる前に下端を読むと最後のページの行を送らずに末尾と
 * 見なしてしまう。件数の行の数（条件に合う全部の数）と、列の見出しの「Select all N loaded
 * tags」の読み込んだ数が等しくなったら描き終えている。件数の行に「 · N loaded」を添える
 * 画面（036 の初めの形）はそれが消えたかで見る。どちらも無い画面（ページが無い）は描き終えて
 * いるものとする。
 */
async function listFullyRendered(page: Page): Promise<boolean> {
  const text = (await countLine(page).textContent({ timeout: waitTimeout })) ?? "";
  if (/ · [\d,]+ loaded$/.test(text)) return false;
  const total = /^([\d,]+)/.exec(text)?.[1];
  const selectAll = page.getByRole("checkbox", {
    name: /^Select (all [\d,]+ loaded tags|the 1 loaded tag)$/,
  });
  if (total === undefined || (await selectAll.count()) === 0) return true;
  const label = (await selectAll.first().getAttribute("aria-label")) ?? "";
  const loaded = /([\d,]+) loaded/.exec(label)?.[1] ?? "1";
  return loaded === total;
}

interface Row {
  scene: string;
  value: string;
  expected: string;
  note: string;
}

test("タグ管理画面を規模のデータで測る", async ({ page, browser }) => {
  const results: Row[] = [];

  const created = await page.request.post("/api/auth/setup", { data: account });
  if (created.status() === 409) {
    const login = await page.request.post("/api/auth/login", { data: account });
    expect(login.status()).toBe(200);
  } else {
    expect(created.status()).toBe(200);
  }
  // ページで返すサーバーは limit=1 で 1 件と totalAll を返す。変更前（ページが無い）は
  // limit を無視して全件を返すので、件数をそこから取る。
  const listed = await page.request.get("/api/tags?limit=1");
  expect(listed.status()).toBe(200);
  const listedBody = (await listed.json()) as TagListBody;
  const tagTotal = listedBody.totalAll ?? listedBody.items.length;
  // 規模のデータが崩れていないこと（タグの数が規模と同じ）を先に確かめる。
  if (scale !== "?") expect(tagTotal).toBe(Number(scale));

  await page.addInitScript(installProbes, rowLinkPrefix);

  // 1. 開いてから最初の行が出るまで（受け入れ条件 1）。読み込みごとに測り直す。
  const loads: {
    firstRow: number;
    api: number;
    apiFirstByte: number;
    longTask: string;
    items: number;
    bodyBytes: number;
    transferBytes: number;
    hasMore: boolean;
  }[] = [];
  for (let run = 0; run < loadRuns; run += 1) {
    await page.goto("about:blank");
    const response = page.waitForResponse(isTagList, { timeout: waitTimeout });
    await page.goto("/tags");
    await rows(page).first().waitFor({ state: "attached", timeout: waitTimeout });
    const opened = await response;
    const timing = opened.request().timing();
    const body = await opened.body();
    const listBody = JSON.parse(body.toString("utf8")) as TagListBody;
    const sizes = await opened.request().sizes();
    const firstRow = await page.evaluate(() => window.__tagsBench?.firstRowAt ?? NaN);
    await settle(page);
    loads.push({
      firstRow,
      api: timing.responseEnd,
      apiFirstByte: timing.responseStart,
      longTask: longTaskText(
        (await longTasksSince(page, 0)).filter((task) => task.start <= firstRow),
      ),
      items: listBody.items.length,
      bodyBytes: body.length,
      transferBytes: sizes.responseBodySize,
      hasMore: listBody.nextCursor !== undefined,
    });
  }
  results.push({
    scene: "開いてから最初の行が出るまで",
    value: `${ms(median(loads.map((load) => load.firstRow)))}（中央値。各回 ${loads
      .map((load) => ms(load.firstRow))
      .join(" / ")}）`,
    expected: "1 秒以内",
    note: `最初の行までの最長のタスク ${loads.map((load) => load.longTask).join(" / ")}`,
  });

  // 2. 開いたときに受け取るタグ（受け入れ条件 2）。応答の大きさは本文（展開後）の
  // バイト数で、補足に転送の大きさ（圧縮されていれば圧縮後）を添える。
  const opened = loads.at(-1);
  if (opened !== undefined) {
    results.push({
      scene: "開いたときに受け取るタグ",
      value: `items ${opened.items.toLocaleString("en-US")} 件・本文 ${bytes(opened.bodyBytes)}`,
      expected: "3 つの規模で同じ数・同じ大きさ（±5%）",
      note: `転送 ${bytes(opened.transferBytes)}、続き（nextCursor）${opened.hasMore ? "あり" : "なし"}。各回の本文 ${loads
        .map((load) => bytes(load.bodyBytes))
        .join(" / ")}`,
    });
  }

  // 内訳（quickstart.md「内訳の切り分け」）: 開いたときの GET /api/tags の応答時間を
  // 描画と分けて出す。
  results.push({
    scene: "GET /api/tags の応答（開いたとき・内訳）",
    value: `${ms(median(loads.map((load) => load.api)))}（中央値）`,
    expected: "—",
    note: `各回 ${loads
      .map((load) => `${ms(load.api)}（最初のバイト ${ms(load.apiFirstByte)}）`)
      .join(" / ")}`,
  });

  // 3. 検索の 1 文字目（受け入れ条件 3）。
  const search = page.getByRole("searchbox", { name: "Search tags" });
  await search.focus();
  const typed = await interact(page, async () => {
    await page.keyboard.type("a");
    await filteredCount(page).waitFor({ timeout: waitTimeout });
  });
  results.push({
    scene: "検索の 1 文字目",
    value: longTaskText(typed.longTasks),
    expected: "0.2 秒を超えない",
    note: `一覧が変わるまで ${ms(typed.elapsed)}`,
  });

  // 4. Esc での取り消し（受け入れ条件 3）。
  const cleared = await interact(page, async () => {
    await page.keyboard.press("Escape");
    await filteredCount(page).waitFor({ state: "detached", timeout: waitTimeout });
  });
  results.push({
    scene: "Esc での取り消し",
    value: longTaskText(cleared.longTasks),
    expected: "0.2 秒を超えない",
    note: `一覧が戻るまで ${ms(cleared.elapsed)}`,
  });

  // 5. 1 件の確定（受け入れ条件 3）。
  const confirmButton = page
    .getByRole("button", { name: "Confirm", exact: true })
    .first();
  // 一覧が見えている行だけを描くときは、先頭の付近に仮の行が無いと「確定する」が
  // DOM に無い。仮の行が描かれるまで文書を送る（送る間は計測に含めない）。名前の順では
  // 英字の語のタグ（すべて確定）が先に並び、30,000 個の規模では仮の行が 15,000 行目の
  // あたりまで出ないので、大きく送り、続きを読み込む間も送り続ける。
  const searchDeadline = Date.now() + scrollStallTimeout * 5;
  while ((await confirmButton.count()) === 0 && Date.now() < searchDeadline) {
    await page.evaluate(() => window.scrollBy(0, 2_000));
    await scrollPosition(page);
  }
  await confirmButton.waitFor({ timeout: waitTimeout });
  const confirmed = await interact(page, async () => {
    await confirmButton.click();
    await page.getByText(/^Confirmed "/).waitFor({ timeout: waitTimeout });
  });
  results.push({
    scene: "1 件の確定",
    value: longTaskText(confirmed.longTasks),
    expected: "0.2 秒を超えない",
    note: `行が差し替わるまで ${ms(confirmed.elapsed)}`,
  });

  // 6. 1 件の改名（受け入れ条件 3）。名前の後ろに語を足し、並びの位置を変えない
  // （ページで読む画面は、読み込んだ範囲の外へ動いた行を一覧から外す）。
  await page.getByRole("button", { name: "Rename", exact: true }).first().click();
  const renameInput = page.getByRole("textbox", { name: /^New name for "/ });
  const renameLabel = (await renameInput.getAttribute("aria-label")) ?? "";
  const oldName = /^New name for "(.*)"$/.exec(renameLabel)?.[1] ?? "tagsbench";
  const newName = `${oldName} renamed ${String(Date.now())}`;
  await renameInput.fill(newName);
  const renamed = await interact(page, async () => {
    await renameInput.press("Enter");
    await page
      .getByRole("link", { name: `${rowLinkPrefix}${newName}`, exact: true })
      .waitFor({ state: "attached", timeout: waitTimeout });
  });
  results.push({
    scene: "1 件の改名",
    value: longTaskText(renamed.longTasks),
    expected: "0.2 秒を超えない",
    note: `行が差し替わるまで ${ms(renamed.elapsed)}`,
  });

  // 7. スクロール（受け入れ条件 4）。開き直した一覧の先頭から、続きを読み込みながら
  // 末尾までホイールで送る。
  results.push(await measureScroll(page));

  // 8. まとめての確定（受け入れ条件 10）。
  results.push(await measureBulkConfirm(page));

  const header = [
    `タグ管理画面の計測: 規模 ${scale}（タグ ${tagTotal.toLocaleString("en-US")} 個・動画 ${Number.isNaN(Number(videos)) ? videos : Number(videos).toLocaleString("en-US")} 本）、${browser.browserType().name()} ${browser.version()}`,
    "",
    "| 場面 | 値 | 期待 | 補足 |",
    "| --- | --- | --- | --- |",
  ];
  const table = [
    ...header,
    ...results.map(
      (row) => `| ${row.scene} | ${row.value} | ${row.expected} | ${row.note} |`,
    ),
  ].join("\n");
  // tagsbench から呼ばれたときは表をファイルに書き、tagsbench が最後に出す。
  if (resultPath === undefined || resultPath === "") console.log(`\n${table}\n`);
  else await writeFile(resultPath, `${table}\n`);
});

interface ScrollPosition {
  top: number;
  atEnd: boolean;
}

/**
 * scrollPosition は一覧を送っている要素の位置を、2 フレーム続けて動かなくなるまで
 * （上限 2 秒）待ってから返す。atEnd は下端に届いているか。
 */
async function scrollPosition(page: Page): Promise<ScrollPosition> {
  return page.evaluate(
    (prefix) =>
      new Promise<ScrollPosition>((resolve) => {
        const target = (): Element => {
          const row = document.querySelector(`a[aria-label^="${prefix}"]`);
          let scroller: Element | null = row?.parentElement ?? null;
          while (scroller !== null) {
            const style = getComputedStyle(scroller);
            if (
              /(auto|scroll)/.test(style.overflowY) &&
              scroller.scrollHeight > scroller.clientHeight
            )
              break;
            scroller = scroller.parentElement;
          }
          return scroller ?? document.scrollingElement ?? document.documentElement;
        };
        const deadline = performance.now() + 2_000;
        let last = -1;
        let still = 0;
        const check = () => {
          const element = target();
          const top = element.scrollTop;
          still = top === last ? still + 1 : 0;
          last = top;
          if (still >= 2 || performance.now() > deadline) {
            resolve({
              top,
              atEnd: top + element.clientHeight >= element.scrollHeight - 1,
            });
            return;
          }
          requestAnimationFrame(check);
        };
        requestAnimationFrame(check);
      }),
    rowLinkPrefix,
  );
}

/**
 * ListLoads は、一覧の応答（先頭のページと続き）を数え、続きが尽きたか（最後の応答に
 * `nextCursor` が無い）を持つ。変更前の画面（ページが無い）は最初の応答で尽きる。
 */
interface ListLoads {
  responses: number;
  loaded: number;
  totalAll: number | undefined;
  exhausted: boolean;
  /** 続きのページの応答時間（`response.timing`）。 */
  moreTimes: number[];
}

async function measureScroll(page: Page): Promise<Row> {
  // 開き直して、先頭のページだけを読んだ状態から始める（前の場面で読んだ続きを持ち越さない）。
  const loads: ListLoads = {
    responses: 0,
    loaded: 0,
    totalAll: undefined,
    exhausted: false,
    moreTimes: [],
  };
  const pending = new Set<Promise<void>>();
  const onResponse = (response: Response) => {
    if (!isTagList(response)) return;
    const task = response
      .json()
      .then((body: TagListBody) => {
        const more = new URL(response.url()).searchParams.has("cursor");
        loads.responses += 1;
        loads.loaded = more ? loads.loaded + body.items.length : body.items.length;
        loads.totalAll = body.totalAll ?? body.items.length;
        loads.exhausted = body.nextCursor === undefined;
        if (more) loads.moreTimes.push(response.request().timing().responseEnd);
      })
      .catch(() => undefined)
      .finally(() => pending.delete(task));
    pending.add(task);
  };
  page.on("response", onResponse);
  await page.goto("about:blank");
  await page.goto("/tags");
  await rows(page).first().waitFor({ state: "attached", timeout: waitTimeout });
  await settle(page);
  await Promise.all([...pending]);

  const first = rows(page).first();
  await first.scrollIntoViewIfNeeded();
  const box = await first.boundingBox();
  await page.mouse.move(box === null ? 640 : box.x + 20, box === null ? 400 : box.y + 5);

  const start = await now(page);
  await page.evaluate(() => {
    const state = window.__tagsBench;
    if (state === undefined) return;
    state.frames = [];
    state.recording = true;
    const tick = (time: number) => {
      state.frames.push(time);
      if (state.recording) requestAnimationFrame(tick);
    };
    requestAnimationFrame(tick);
  });

  // 続きが尽きて下端に着くまで送る。page.mouse.wheel はスクロールが済むのを待たないので、
  // 送るたびに位置が 2 フレーム続けて動かなくなるまで待ってから読む。仮想化した一覧は
  // 送るたびに高さが変わりうるので、末尾は「続きが尽き、下端に届いている」ことで決める。
  // 下端で続きを待つ間も送り続ける（利用者がホイールを回し続ける場面）。位置も読み込んだ
  // 行も変わらないまま scrollStallTimeout が経ったら諦める。
  // 続きが尽きたことは応答で分かるが、最後のページの行が描かれるのはその後なので、尽きて
  // 描き終えたことを位置を読む前に確かめ、その後に読んだ位置が下端のときだけ末尾とする。
  let previous = -1;
  let previousResponses = loads.responses;
  let lastProgress = Date.now();
  let atEnd = false;
  let complete = false;
  let steps = 0;
  while (!(complete && atEnd)) {
    if (Date.now() - lastProgress > scrollStallTimeout) break;
    await page.mouse.wheel(0, 400);
    steps += 1;
    complete = loads.exhausted && pending.size === 0 && (await listFullyRendered(page));
    const position = await scrollPosition(page);
    atEnd = position.atEnd;
    if (position.top !== previous || loads.responses !== previousResponses) {
      lastProgress = Date.now();
    }
    previous = position.top;
    previousResponses = loads.responses;
  }
  page.off("response", onResponse);

  // 最後のスクロールの描画を含むフレームまで記録してから止める。
  const frames = await page.evaluate(
    () =>
      new Promise<number[]>((resolve) => {
        const state = window.__tagsBench;
        if (state === undefined) {
          resolve([]);
          return;
        }
        requestAnimationFrame(() =>
          requestAnimationFrame(() => {
            state.recording = false;
            resolve([...state.frames]);
          }),
        );
      }),
  );
  await settle(page);
  const longTasks = await longTasksSince(page, start);
  const intervals = frames.slice(1).map((time, index) => time - (frames[index] ?? time));
  const slow = intervals.map((interval) => interval > 50);
  const slowCount = slow.filter(Boolean).length;
  const consecutive = slow.slice(1).filter((value, index) => value && slow[index]).length;
  return {
    scene: "スクロール",
    value: `50 ms を超えるフレームが続いた回数 ${String(consecutive)}（最長のフレーム ${ms(maxOf(intervals))}）`,
    expected: "50 ms を超えるフレームが 2 つ続かない",
    note: `フレーム ${String(intervals.length)} 個、50 ms 超 ${String(slowCount)} 個、${complete && atEnd ? "末尾" : "末尾に届かず"}の位置 ${ms(previous).replace(" ms", " px")}（ホイール ${steps.toLocaleString("en-US")} 回）、読み込んだ行 ${loads.loaded.toLocaleString("en-US")} / ${(loads.totalAll ?? 0).toLocaleString("en-US")}、続きの要求 ${String(loads.moreTimes.length)} 回（応答 中央値 ${ms(median(loads.moreTimes))} / 最長 ${ms(maxOf(loads.moreTimes))}）、最長のタスク ${longTaskText(longTasks)}`,
  };
}

async function measureBulkConfirm(page: Page): Promise<Row> {
  const scene = "まとめての確定";
  const expected = "0.2 秒を超えない";
  // 開き直す。スクロールの場面で全部の行を読み込んだままだと、読み込んだ行をすべて
  // 選ぶ操作が上限で使えない。
  await page.goto("about:blank");
  await page.goto("/tags");
  await rows(page).first().waitFor({ state: "attached", timeout: waitTimeout });
  // 「Tentative only」はトップバーの「Filter」の吹き出しの中のチェック（変更前は操作の行の
  // 押すボタン）。
  const tentativeButton = page.getByRole("button", { name: "Tentative only" });
  if ((await tentativeButton.count()) > 0) {
    await tentativeButton.click();
  } else {
    await page.getByRole("button", { name: /^Filter/ }).click();
    await page.getByRole("checkbox", { name: /^Tentative only/ }).click();
    await page.keyboard.press("Escape");
  }
  await filteredCount(page).waitFor({ timeout: waitTimeout });
  // 変更前は「見えているものをすべて選ぶ」、ページで読む画面は「読み込んだ行をすべて選ぶ」。
  const selectAll = page.getByRole("checkbox", {
    name: /^Select (all shown tags|all [\d,]+ loaded tags|the [\d,]+ loaded tag)$/,
  });
  try {
    await selectAll.waitFor({ timeout: 5_000 });
  } catch {
    return {
      scene,
      value: "測れない",
      expected,
      note: "画面にまとめて選ぶ操作（「Select all shown tags」「Select all N loaded tags」）が無い",
    };
  }
  const shown = (await filteredCount(page).textContent()) ?? "";
  await selectAll.click();
  const confirm = page
    .getByRole("region", { name: "Selected tags" })
    .getByRole("button", { name: "Confirm", exact: true });
  const bulk = await interact(page, async () => {
    await confirm.click();
    // 変更前は全部の仮のタグを読み込んでいるので空の表示が出る。ページで読む画面は
    // 読み込んだ分だけを確定し、結果の通知が出る。
    await page
      .getByText("No tentative tags", { exact: true })
      .or(page.getByText(/^Confirmed [\d,]+ tags?/))
      .first()
      .waitFor({ timeout: waitTimeout });
  });
  return {
    scene,
    value: longTaskText(bulk.longTasks),
    expected,
    note: `「Tentative only」で ${shown}。仮の目印が消えるまで ${ms(bulk.elapsed)}`,
  };
}
