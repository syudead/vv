import { writeFile } from "node:fs/promises";

import { expect, test, type Locator, type Page, type Response } from "@playwright/test";

// Write per-scenario Markdown results to TAGSBENCH_RESULT. Measure long tasks and frame intervals in-page, using accessible names to remain independent of virtualization.

const scale = process.env.TAGSBENCH_SCALE ?? "?";
const videos = process.env.TAGSBENCH_VIDEOS ?? "?";
const resultPath = process.env.TAGSBENCH_RESULT;

const account = { username: "tagsbench", password: "tagsbench-password" } as const;

/** Accessible-name prefix from tags.row.open in src/i18n/en.ts. */
const rowLinkPrefix = "Open the library filtered by ";

const loadRuns = 3;
/** Allow the slower baseline screen to finish at 3,000 tags. */
const waitTimeout = 120_000;
/** Stop when neither scroll position nor loaded-row count advances for this duration. */
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

/** Install probes with addInitScript before page loading. */
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

/** Wait two frames plus a delay so late long-task entries are collected. */
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
  /** Time to visible result, including Playwright round trips. */
  elapsed: number;
  longTasks: LongTaskEntry[];
}

/** Collect long tasks during the action and its result wait. */
async function interact(page: Page, action: () => Promise<void>): Promise<Interaction> {
  const start = await now(page);
  const started = Date.now();
  await action();
  const elapsed = Date.now() - started;
  await settle(page);
  return { elapsed, longTasks: await longTasksSince(page, start) };
}

/** Exclude merge-candidate searches containing q from list responses. */
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

/** Accept filtered counts with the optional legacy loaded-count suffix. */
function filteredCount(page: Page): Locator {
  return page
    .getByRole("status")
    .filter({ hasText: / of [\d,]+ tags?( · [\d,]+ loaded)?$/ });
}

/** Accept total or filtered counts with the optional legacy loaded-count suffix. */
function countLine(page: Page): Locator {
  return page
    .getByRole("status")
    .filter({ hasText: /[\d,]+ tags?( · [\d,]+ loaded)?$/ })
    .first();
}

/** A completed response does not imply rendered rows. Compare the loaded and total counts (or the legacy suffix) before deciding the bottom is reached. */
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
  // Paginated servers expose totalAll at limit=1; older servers ignore limit, so use their full result length.
  const listed = await page.request.get("/api/tags?limit=1");
  expect(listed.status()).toBe(200);
  const listedBody = (await listed.json()) as TagListBody;
  const tagTotal = listedBody.totalAll ?? listedBody.items.length;
  // Verify the seed tag count before measuring.
  if (scale !== "?") expect(tagTotal).toBe(Number(scale));

  await page.addInitScript(installProbes, rowLinkPrefix);

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

  // Report decompressed body bytes separately from transferred bytes, which may be compressed.
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

  // Separate API response time from rendering time.
  results.push({
    scene: "GET /api/tags の応答（開いたとき・内訳）",
    value: `${ms(median(loads.map((load) => load.api)))}（中央値）`,
    expected: "—",
    note: `各回 ${loads
      .map((load) => `${ms(load.api)}（最初のバイト ${ms(load.apiFirstByte)}）`)
      .join(" / ")}`,
  });

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

  const confirmButton = page
    .getByRole("button", { name: "Confirm", exact: true })
    .first();
  // Scroll outside the measured interval until a tentative virtual row mounts; at 30,000 tags the first may be near row 15,000.
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

  // Append to the name to preserve sort position; paginated lists drop renamed rows that move outside the loaded range.
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

  // Start from a fresh first page and wheel through all remaining pages.
  results.push(await measureScroll(page));

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
  // Let the tagsbench parent print the saved table at the end.
  if (resultPath === undefined || resultPath === "") console.log(`\n${table}\n`);
  else await writeFile(resultPath, `${table}\n`);
});

interface ScrollPosition {
  top: number;
  atEnd: boolean;
}

/** Wait for two stable frames, up to two seconds, before reading scroll position and atEnd. */
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

/** Track loaded rows and exhaustion from nextCursor; nonpaginated servers finish on their first response. */
interface ListLoads {
  responses: number;
  loaded: number;
  totalAll: number | undefined;
  exhausted: boolean;
  /** Continuation response timings from response.timing. */
  moreTimes: number[];
}

async function measureScroll(page: Page): Promise<Row> {
  // Reload so this scenario starts with only the first page.
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

  // Wheel does not await scrolling, and virtualization can change list height. Declare the end only after pagination is exhausted, final rows are rendered, and the stable position reaches the bottom; stop stalled progress at scrollStallTimeout.
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

  // Include the frame that renders the final scroll.
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
  // Reload to avoid exceeding the bulk-selection limit after loading every row.
  await page.goto("about:blank");
  await page.goto("/tags");
  await rows(page).first().waitFor({ state: "attached", timeout: waitTimeout });
  // Support Tentative only in either the Filter popover or the legacy action row.
  const tentativeButton = page.getByRole("button", { name: "Tentative only" });
  if ((await tentativeButton.count()) > 0) {
    await tentativeButton.click();
  } else {
    await page.getByRole("button", { name: /^Filter/ }).click();
    await page.getByRole("checkbox", { name: /^Tentative only/ }).click();
    await page.keyboard.press("Escape");
  }
  await filteredCount(page).waitFor({ timeout: waitTimeout });
  // Support both legacy visible-row and current loaded-row selection labels.
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
    // Legacy screens confirm every tentative tag and become empty; paginated screens confirm loaded rows and show a result toast.
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
