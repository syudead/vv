import type { Zoom } from "../../preferences/viewPreferences";
import type { FaceCardSize } from "../../ui/patterns/card-grid";

const storageKey = "vv.people.zoom";

export const faceSizes: Record<Zoom, FaceCardSize> = {
  0: "face-sm",
  1: "face",
  2: "face-lg",
  3: "face-xl",
};

/** The people grid's card size, remembered per browser apart from the library's. */
export function readPeopleZoom(storage?: Storage): Zoom {
  try {
    const value = (storage ?? window.localStorage).getItem(storageKey);
    return value === "0" || value === "2" || value === "3" ? (Number(value) as Zoom) : 1;
  } catch {
    return 1;
  }
}

export function writePeopleZoom(zoom: Zoom, storage?: Storage): void {
  try {
    (storage ?? window.localStorage).setItem(storageKey, String(zoom));
  } catch {
    // The size still applies to this visit when it cannot be stored.
  }
}
