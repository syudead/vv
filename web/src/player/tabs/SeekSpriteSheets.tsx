import { useCallback, useEffect, useRef } from "react";

import type { SeekThumbnailSprite } from "../../api/client";
import { t } from "../../i18n";
import { Skeleton } from "../../ui/shadcn/skeleton";
import { PanelSection, ReadFailure } from "./panelParts";
import { type SpriteSheets, useSpriteSheets } from "./useSpriteFrame";

/** Read a sheet once it comes near the viewport; at once without an observer. */
function useSheetObserver(requestSheet: (index: number) => void) {
  const observer = useRef<IntersectionObserver | null>(null);
  useEffect(() => () => observer.current?.disconnect(), []);
  return useCallback(
    (element: HTMLElement | null) => {
      if (element === null) return;
      const sheet = Number(element.dataset.sheet);
      if (typeof IntersectionObserver === "undefined") {
        requestSheet(sheet);
        return;
      }
      observer.current ??= new IntersectionObserver(
        (entries) => {
          for (const entry of entries) {
            if (!entry.isIntersecting) continue;
            observer.current?.unobserve(entry.target);
            requestSheet(Number((entry.target as HTMLElement).dataset.sheet));
          }
        },
        { rootMargin: "50%" },
      );
      observer.current.observe(element);
    },
    [requestSheet],
  );
}

/** The seek sprite's sheets as they are, for viewing; a sheet is read as it comes into view. */
export default function SeekSpriteSheets({ spriteUrl }: { spriteUrl: string }) {
  const sheets = useSpriteSheets(spriteUrl, true);
  const text = t.player.tabs;
  return (
    <PanelSection title={text.seekSprite}>
      {sheets.failed ? (
        <ReadFailure text={text.seekSpriteFailed} onRetry={sheets.retry} />
      ) : sheets.sprite === null ? (
        <Skeleton
          role="status"
          aria-label={text.seekSpriteLoading}
          className="aspect-video w-full"
        />
      ) : (
        <Sheets sheets={sheets} sprite={sheets.sprite} />
      )}
    </PanelSection>
  );
}

function Sheets({
  sheets,
  sprite,
}: {
  sheets: SpriteSheets;
  sprite: SeekThumbnailSprite;
}) {
  const observe = useSheetObserver(sheets.requestSheet);
  const aspectRatio = `${String(sprite.columns * sprite.frameWidth)} / ${String(sprite.rows * sprite.frameHeight)}`;
  return (
    <>
      <ul aria-label={t.player.tabs.seekSprite} className="flex flex-col gap-2">
        {sprite.sheets.map((_, index) => {
          const url = sheets.sheetUrl(index);
          return (
            <li
              key={index}
              ref={observe}
              data-sheet={index}
              className="overflow-hidden rounded-md bg-muted"
              style={{ aspectRatio }}
            >
              {url !== null && (
                <img
                  src={url}
                  alt={t.player.tabs.seekSpriteSheet(index + 1, sprite.sheets.length)}
                  className="size-full"
                />
              )}
            </li>
          );
        })}
      </ul>
      {sheets.sheetFailed && (
        <ReadFailure text={t.player.tabs.seekSpriteFailed} onRetry={sheets.retry} />
      )}
    </>
  );
}
