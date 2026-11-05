const storageKey = "vv.assistantThreadCleared.v1";

/** When Clear thread was last pressed on this device, in epoch ms; 0 if never, invalid or unreadable. */
export function readThreadClearedAt(storage?: Storage): number {
  try {
    const value = Number((storage ?? window.localStorage).getItem(storageKey));
    return Number.isFinite(value) && value > 0 ? value : 0;
  } catch {
    return 0;
  }
}

/** A failed write is ignored; the panel then rebuilds the cleared turns after a reload. */
export function writeThreadClearedAt(at: number, storage?: Storage): void {
  try {
    (storage ?? window.localStorage).setItem(storageKey, String(at));
  } catch {
    // Storage may be blocked; the preference is a convenience.
  }
}
