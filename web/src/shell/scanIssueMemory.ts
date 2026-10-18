import {
  listCurrentScanIssues,
  MAX_SCAN_ISSUE_PAGE_SIZE,
  type ScanIssue,
} from "../api/client";

/**
 * Compare failure snapshots because partial status can include carried-over failures that should
 * not notify again.
 */
let lastRead: { scanId: number; keys: ReadonlySet<string> } | null = null;

/** A changed failure kind is a new failure even for the same path. */
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

/** Without a previous snapshot, treat failures as new. */
export function hasNewFailure(
  keys: ReadonlySet<string>,
  previous: ReadonlySet<string> | null,
): boolean {
  if (previous === null) return true;
  for (const key of keys) if (!previous.has(key)) return true;
  return false;
}

/**
 * Read all issue pages; return null if the scan changes. Reject stuck cursors instead of silently
 * missing new failures.
 */
export async function readFailureKeys(
  expected: number,
  signal: AbortSignal,
): Promise<Set<string> | null> {
  const items: ScanIssue[] = [];
  let cursor: string | undefined;
  for (;;) {
    const result = await listCurrentScanIssues({
      limit: MAX_SCAN_ISSUE_PAGE_SIZE,
      cursor,
      signal,
    });
    if (result === null || result.scanId !== expected) return null;
    items.push(...result.items);
    if (result.nextCursor === undefined) break;
    if (result.nextCursor === cursor)
      throw new Error("scan issue cursor did not advance");
    cursor = result.nextCursor;
  }
  return failureKeys(items);
}

export function previousFailureKeys(): ReadonlySet<string> | null {
  return lastRead?.keys ?? null;
}

export function rememberFailureKeys(scanId: number, keys: ReadonlySet<string>): void {
  // Do not let an old scan's delayed response overwrite a newer failure snapshot.
  if (lastRead !== null && lastRead.scanId > scanId) return;
  lastRead = { scanId, keys };
}

export function forgetFailureKeys(): void {
  lastRead = null;
}
