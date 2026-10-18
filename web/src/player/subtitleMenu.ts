import videojs from "video.js";

/** Subtitle labels are user data, so restore raw track labels after video.js localization. Keep the Off item localized. */

interface MenuItemLike {
  el(): Element;
}

interface TrackMenuItemLike extends MenuItemLike {
  track: { label?: string };
}

type Constructor<T> = abstract new (...args: never[]) => T;

export const subtitlesButtonName = "subtitlesButton";

let registered = false;

/** Register once before creating the player. */
export function registerSubtitlesButton(): void {
  if (registered) return;
  const SubsCapsButton = videojs.getComponent(
    "SubsCapsButton",
  ) as unknown as Constructor<{
    createItems(): MenuItemLike[];
  }>;
  const TextTrackMenuItem = videojs.getComponent(
    "TextTrackMenuItem",
  ) as unknown as Constructor<TrackMenuItemLike>;
  const OffTextTrackMenuItem = videojs.getComponent(
    "OffTextTrackMenuItem",
  ) as unknown as Constructor<object>;

  class SubtitlesButton extends SubsCapsButton {
    override createItems(): MenuItemLike[] {
      const items = super.createItems();
      for (const item of items) {
        const offItem: unknown = item;
        if (
          !(item instanceof TextTrackMenuItem) ||
          offItem instanceof OffTextTrackMenuItem
        ) {
          continue;
        }
        const text = item.el().querySelector(".vjs-menu-item-text");
        const label = item.track.label;
        if (text !== null && label !== undefined) text.textContent = label;
      }
      return items;
    }
  }

  videojs.registerComponent(
    "SubtitlesButton",
    SubtitlesButton as unknown as Parameters<typeof videojs.registerComponent>[1],
  );
  registered = true;
}
