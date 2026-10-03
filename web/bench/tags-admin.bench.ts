import { writeFile } from "node:fs/promises";

import { expect, test, type Locator, type Page } from "@playwright/test";

// タグ管理画面（/tags）を規模のデータで測る（specs/036-tag-admin-scale/quickstart.md の
// 「場面と期待」）。`go run ./scripts/tagsbench` から呼ばれ、場面ごとの値を Markdown の表で
// TAGSBENCH_RESULT に書く。
//
// Long Task とフレーム時間はページの中から読む（PerformanceObserver の longtask と
// requestAnimationFrame の間隔）。画面の要素は読み上げ名で引くので、一覧の描き方
// （仮想化など）が変わっても同じ場面を測れる。

const scale = process.env.TAGSBENCH_SCALE ?? "?";
const resultPath = process.env.TAGSBENCH_RESULT;

const account = { username: "tagsbench", password: "tagsbench-password" } as const;

/** 行の名前のリンクの読み上げ名の頭（web/src/i18n/en.ts の tags.row.open）。 */
const rowLinkPrefix = "Open the library filtered by ";
/** 最初の行を開いてから何回測るか。 */
const loadRuns = 3;
/** 待つ上限。3,000 個の規模で遅い変更前の画面でも収まる長さにする。 */
const waitTimeout = 120_000;

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

function rows(page: Page): Locator {
  return page.getByRole("link", { name: new RegExp(`^${rowLinkPrefix}`) });
}

/** countLine は件数の行（「1,000 tags」「12 of 1,000 tags」）である。 */
function filteredCount(page: Page): Locator {
  return page.getByRole("status").filter({ hasText: / of [\d,]+ tags?$/ });
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
  const listed = await page.request.get("/api/tags");
  expect(listed.status()).toBe(200);
  const tagTotal = ((await listed.json()) as { items: unknown[] }).items.length;
  // 規模のデータが崩れていないこと（タグの数が規模と同じ）を先に確かめる。
  if (scale !== "?") expect(tagTotal).toBe(Number(scale));

  await page.addInitScript(installProbes, rowLinkPrefix);

  // 1. 開いてから最初の行が出るまで（受け入れ条件 1）。読み込みごとに測り直す。
  const loads: {
    firstRow: number;
    api: number;
    apiFirstByte: number;
    longTask: string;
  }[] = [];
  for (let run = 0; run < loadRuns; run += 1) {
    await page.goto("about:blank");
    const response = page.waitForResponse(
      (r) => new URL(r.url()).pathname === "/api/tags" && r.request().method() === "GET",
      { timeout: waitTimeout },
    );
    await page.goto("/tags");
    await rows(page).first().waitFor({ state: "attached", timeout: waitTimeout });
    const timing = (await response).request().timing();
    const firstRow = await page.evaluate(() => window.__tagsBench?.firstRowAt ?? NaN);
    await settle(page);
    loads.push({
      firstRow,
      api: timing.responseEnd,
      apiFirstByte: timing.responseStart,
      longTask: longTaskText(
        (await longTasksSince(page, 0)).filter((task) => task.start <= firstRow),
      ),
    });
  }
  results.push({
    scene: "開いてから最初の行が出るまで",
    value: `${ms(median(loads.map((load) => load.firstRow)))}（中央値。各回 ${loads
      .map((load) => ms(load.firstRow))
      .join(" / ")}）`,
    expected: "1 秒以内",
    note: `GET /api/tags の応答 ${loads
      .map((load) => `${ms(load.api)}（最初のバイト ${ms(load.apiFirstByte)}）`)
      .join(
        " / ",
      )}。最初の行までの最長のタスク ${loads.map((load) => load.longTask).join(" / ")}`,
  });

  // 2. 検索の 1 文字目（受け入れ条件 2）。
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

  // 3. Esc での取り消し（受け入れ条件 2）。
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

  // 4. 1 件の確定（受け入れ条件 2）。
  const confirmButton = page
    .getByRole("button", { name: "Confirm", exact: true })
    .first();
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

  // 5. 1 件の改名（受け入れ条件 2）。
  await page.getByRole("button", { name: "Rename", exact: true }).first().click();
  const renameInput = page.getByRole("textbox", { name: /^New name for "/ });
  const newName = `tagsbench renamed ${String(Date.now())}`;
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

  // 6. スクロール（受け入れ条件 3）。先頭から末尾までホイールで送る。
  results.push(await measureScroll(page));

  // 7. まとめての確定（受け入れ条件 9）。
  results.push(await measureBulkConfirm(page));

  const header = [
    `タグ管理画面の計測: 規模 ${scale}（タグ ${tagTotal.toLocaleString("en-US")} 個）、${browser.browserType().name()} ${browser.version()}`,
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

async function measureScroll(page: Page): Promise<Row> {
  await page.evaluate(() => {
    window.scrollTo(0, 0);
    for (const element of document.querySelectorAll("*")) {
      if (element.scrollTop > 0) element.scrollTop = 0;
    }
  });
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

  // 末尾に着くまで送る。page.mouse.wheel はスクロールが済むのを待たないので、送るたびに
  // 位置が 2 フレーム続けて動かなくなるまで待ってから読む。仮想化した一覧は送るたびに
  // 高さが変わりうるので、末尾は「下端に届いている」ことで決める。
  let stalls = 0;
  let previous = -1;
  let atEnd = false;
  for (let step = 0; step < 5000 && !atEnd && stalls < 20; step += 1) {
    await page.mouse.wheel(0, 400);
    const position = await scrollPosition(page);
    atEnd = position.atEnd;
    stalls = position.top === previous ? stalls + 1 : 0;
    previous = position.top;
  }

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
    note: `フレーム ${String(intervals.length)} 個、50 ms 超 ${String(slowCount)} 個、${atEnd ? "末尾" : "末尾に届かず"}の位置 ${ms(previous).replace(" ms", " px")}、最長のタスク ${longTaskText(longTasks)}`,
  };
}

async function measureBulkConfirm(page: Page): Promise<Row> {
  const scene = "まとめての確定";
  const expected = "0.2 秒を超えない";
  await page.evaluate(() => window.scrollTo(0, 0));
  const tentativeOnly = page.getByRole("button", { name: "Tentative only" });
  await tentativeOnly.click();
  await filteredCount(page).waitFor({ timeout: waitTimeout });
  const selectAll = page.getByRole("checkbox", { name: "Select all shown tags" });
  try {
    await selectAll.waitFor({ timeout: 5_000 });
  } catch {
    return {
      scene,
      value: "測れない",
      expected,
      note: "画面にまとめて選ぶ操作（「Select all shown tags」）が無い",
    };
  }
  const shown = (await filteredCount(page).textContent()) ?? "";
  await selectAll.click();
  const confirm = page
    .getByRole("region", { name: "Selected tags" })
    .getByRole("button", { name: "Confirm", exact: true });
  const bulk = await interact(page, async () => {
    await confirm.click();
    await page
      .getByText("No tentative tags", { exact: true })
      .waitFor({ timeout: waitTimeout });
  });
  return {
    scene,
    value: longTaskText(bulk.longTasks),
    expected,
    note: `「Tentative only」で ${shown}。仮の目印が消えるまで ${ms(bulk.elapsed)}`,
  };
}
