import {
  listCurrentScanIssues,
  MAX_SCAN_ISSUE_PAGE_SIZE,
  type ScanIssue,
} from "../api/client";

/**
 * 最後に読んだ、失敗の項目の一覧である。自動の取り込みの `partial` は、前の取り込みから
 * 持ち越した失敗も数えるので、状態だけでは古い失敗をまた知らせてしまう。新しい失敗かどうかは
 * 画面が、この一覧と比べて決める（specs/042-folder-watch-import/ui-design.md「Bottom-right
 * indicator and result notice」。問題の一覧に項目ごとの「新しい」印は無い）。
 */
let lastRead: { scanId: number; keys: ReadonlySet<string> } | null = null;

/** pageLimit は、1回の読み込みで辿る一覧のページ数の上限である（失敗が際限なく多いとき）。 */
const pageLimit = 25;

/** failureKeys は、失敗の項目の（道筋, 種類）の組である。種類が変われば別の失敗になる。 */
export function failureKeys(items: readonly ScanIssue[]): Set<string> {
  const keys = new Set<string>();
  for (const item of items) {
    if (item.severity !== "failed") continue;
    for (const kind of item.kinds) {
      keys.add(
        [String(item.folder.rootId), item.folder.path, item.fileName, kind].join("\0"),
      );
    }
  }
  return keys;
}

/** hasNewFailure は、前に読んだ一覧に無い失敗があるかを返す。前の一覧が無ければ true。 */
export function hasNewFailure(
  keys: ReadonlySet<string>,
  previous: ReadonlySet<string> | null,
): boolean {
  if (previous === null) return true;
  for (const key of keys) if (!previous.has(key)) return true;
  return false;
}

/**
 * readFailureKeys は直近の取り込みの問題を最後まで読み、失敗の組を返す。読んでいる途中で
 * 取り込みが替わった（`scanId` が `expected` と違う）ときは null を返す。
 */
export async function readFailureKeys(
  expected: number,
  signal: AbortSignal,
): Promise<Set<string> | null> {
  const items: ScanIssue[] = [];
  let cursor: string | undefined;
  for (let page = 0; page < pageLimit; page += 1) {
    const result = await listCurrentScanIssues({
      limit: MAX_SCAN_ISSUE_PAGE_SIZE,
      cursor,
      signal,
    });
    if (result === null || result.scanId !== expected) return null;
    items.push(...result.items);
    cursor = result.nextCursor;
    if (cursor === undefined) break;
  }
  return failureKeys(items);
}

/** previousFailureKeys は前に読んだ一覧を返す。 */
export function previousFailureKeys(): ReadonlySet<string> | null {
  return lastRead?.keys ?? null;
}

/** rememberFailureKeys は読んだ一覧を、次の比べる相手として覚える。 */
export function rememberFailureKeys(scanId: number, keys: ReadonlySet<string>): void {
  lastRead = { scanId, keys };
}

/** forgetFailureKeys は覚えた一覧を捨てる（検査用）。 */
export function forgetFailureKeys(): void {
  lastRead = null;
}
