import { Link } from "react-router";

import type { Video } from "../api/client";
import { personFaceSrc } from "../api/credits";
import { creditDisplayName } from "../credits/displayName";
import { t } from "../i18n";
import { useHoverOpen } from "../library/useHoverOpen";
import { cn } from "../lib/cn";
import { PersonFace } from "../ui/PersonFace";
import { Popover, PopoverAnchor, PopoverContent } from "../ui/shadcn/popover";
import { VideoThumbnailCredit, VideoThumbnailLayer } from "../ui/VideoThumbnail";

type Performer = NonNullable<Video["cardCredits"]>["performers"][number];

const SHOWN_FACES = 3;

/** Hovering a face pops that one performer large; pressing it opens the person. */
function PerformerFace({
  performer,
  selectionMode,
  onPreviewCancel,
}: {
  performer: Performer;
  selectionMode: boolean;
  onPreviewCancel: () => void;
}) {
  const { open, setOpen, triggerHandlers, contentHandlers } =
    useHoverOpen(!selectionMode);
  const name = creditDisplayName(performer.person);
  const src = personFaceSrc({
    id: performer.person.id,
    imageRevision: performer.imageRevision,
  });
  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverAnchor asChild>
        <Link
          to={`/people/${String(performer.person.id)}`}
          aria-label={name}
          // In selection mode a press falls through to the card link, which toggles selection.
          tabIndex={selectionMode ? -1 : undefined}
          className={cn(
            "rounded-full",
            selectionMode ? "pointer-events-none" : "pointer-events-auto",
          )}
          onPointerEnter={(event) => {
            onPreviewCancel();
            triggerHandlers.onPointerEnter(event);
          }}
          onPointerLeave={triggerHandlers.onPointerLeave}
        >
          <PersonFace src={src} name={name} shape="mark" />
        </Link>
      </PopoverAnchor>
      {/* Hover opening never moves focus. */}
      <PopoverContent
        side="bottom"
        align="start"
        className="flex w-face-popover flex-col gap-2 p-2"
        onOpenAutoFocus={(event) => event.preventDefault()}
        onCloseAutoFocus={(event) => event.preventDefault()}
        {...contentHandlers}
      >
        <PersonFace src={src} name={name} shape="profile" />
        <p className="truncate text-sm text-foreground" title={name}>
          {name}
        </p>
      </PopoverContent>
    </Popover>
  );
}

/**
 * The code at the top left, moving right of the selection check whenever the check shows, and
 * performer faces and the label at the bottom left.
 */
export function CardCredits({
  video,
  selectable,
  selected,
  selectionMode,
  onPreviewCancel,
}: {
  video: Video;
  selectable: boolean;
  selected: boolean;
  selectionMode: boolean;
  onPreviewCancel: () => void;
}) {
  const credits = video.cardCredits;
  if (credits === undefined) return null;
  const faces = credits.performers.slice(0, SHOWN_FACES);
  const more = credits.performers.length - faces.length;
  const organization = credits.organization;
  const checkShown = selectable && (selectionMode || selected);
  return (
    <VideoThumbnailLayer>
      {credits.code !== undefined && (
        <VideoThumbnailCredit
          corner="top-start"
          className={cn(
            "tabular-nums transition-all duration-150",
            checkShown
              ? "left-9"
              : selectable &&
                  "group-focus-within:left-9 group-hover:left-9 [@media(hover:none)]:left-9",
          )}
        >
          {credits.code}
        </VideoThumbnailCredit>
      )}
      {(faces.length > 0 || organization !== undefined) && (
        // The faces stop and never start the hover preview.
        <VideoThumbnailCredit corner="bottom-start" data-preview-checkbox="true">
          {faces.map((performer) => (
            <PerformerFace
              key={performer.person.id}
              performer={performer}
              selectionMode={selectionMode}
              onPreviewCancel={onPreviewCancel}
            />
          ))}
          {more > 0 && (
            <span className="shrink-0 tabular-nums">
              {t.list.card.morePerformers(more)}
            </span>
          )}
          {organization !== undefined && (
            <span className="truncate" title={organization.name}>
              {organization.name}
            </span>
          )}
        </VideoThumbnailCredit>
      )}
    </VideoThumbnailLayer>
  );
}
