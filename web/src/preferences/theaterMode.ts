const storageKey = "vv.theater.v1";

/** Return off if storage is absent, invalid or unreadable. */
export function readTheaterMode(storage?: Storage): boolean {
  try {
    return (storage ?? window.localStorage).getItem(storageKey) === "true";
  } catch {
    return false;
  }
}

/** Report whether the value was stored, so callers can keep it in memory instead. */
export function writeTheaterMode(on: boolean, storage?: Storage): boolean {
  try {
    (storage ?? window.localStorage).setItem(storageKey, on ? "true" : "false");
    return true;
  } catch {
    return false;
  }
}
