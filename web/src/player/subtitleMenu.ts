import videojs from "video.js";

/**
 * 字幕ボタンの部品（specs/028-sidecar-subtitles research.md R-10）。
 *
 * video.js の `SubsCapsButton` は、メニューの項目の文字をプレイヤーの言語の表
 * （`playerDictionary()`）に通す。字幕のラベルはファイル名から来る利用者のデータなので、
 * 表の鍵と同じ名前（`movie.Mute.srt` の `Mute` など）だと表の文言に置き換わってしまう。
 * この部品はメニューを作るたびに、字幕の項目の文字をトラックのラベルそのものに戻す。
 * 「オフ」の項目はカタログの文言なので、そのまま表に通す。
 */

interface MenuItemLike {
  el(): Element;
}

interface TrackMenuItemLike extends MenuItemLike {
  track: { label?: string };
}

type Constructor<T> = abstract new (...args: never[]) => T;

/** subtitlesButtonName は操作バーの `children` に書く字幕ボタンの名前である。 */
export const subtitlesButtonName = "subtitlesButton";

let registered = false;

/**
 * registerSubtitlesButton は字幕ボタンの部品を video.js に登録する。プレイヤーを作る前に呼ぶ。
 * 2 回目からは何もしない。
 */
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
        // 「オフ」の項目（OffTextTrackMenuItem）はカタログの文言なので表に通したままにする。
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
