import { type ReactNode, useCallback, useMemo } from "react";

import type { Video } from "../api/client";
import { isOrganizable, itemKey, itemVideo, itemVideos } from "../api/libraryItems";
import type { VideosState } from "../api/useVideos";
import { useAudience } from "../auth/audience";
import { t } from "../i18n";
import type { Zoom } from "../preferences/viewPreferences";
import { CardGrid } from "../ui/patterns/card-grid";
import { CardDragLayer, DraggableCard } from "../videoList/cardDrag";
import { liftedKeys } from "../videoList/cardDragRules";
import { useOrderedItemDrag } from "../videoList/OrderedItemFace";
import { useUndoToast } from "../videoList/undoToast";
import { useVideoDrops } from "../videoList/useVideoDrops";

export default function FolderVideoGrid(props: FolderVideoGridProps) {
  const owner = useAudience() === "owner";
  if (!owner) {
    return (
      <CardGrid size={props.zoom}>
        {itemVideos(props.videos.items).map(props.card)}
      </CardGrid>
    );
  }
  return <OwnerFolderVideoGrid {...props} />;
}

interface FolderVideoGridProps {
  videos: VideosState;
  zoom: Zoom;
  card: (video: Video) => ReactNode;
  /** Keys of the selected videos; pressing one lifts them all in display order. */
  selectedKeys: readonly string[];
  /** Called on lift to stop hover previews. */
  onLift: () => void;
  /** Called after a drop is accepted, to clear the selection. */
  onAccepted: () => void;
}

function OwnerFolderVideoGrid({
  videos,
  zoom,
  card,
  selectedKeys,
  onLift,
  onAccepted,
}: FolderVideoGridProps) {
  const { refreshInPlace } = videos;
  const undo = useUndoToast(refreshInPlace);
  const drops = useVideoDrops({
    items: videos.items,
    keyOf: itemKey,
    videoOf: itemVideo,
    from: null,
    groupItem: null,
    reread: refreshInPlace,
    onAccepted,
    undo,
  });
  const shown = drops.shown;
  // Narrow the type to draggable items; folders never receive smart-folder entries.
  const organizable = useMemo(() => shown.filter(isOrganizable), [shown]);
  const { order, nameOf, preview } = useOrderedItemDrag(organizable, itemKey, false);
  const liftedFor = useCallback(
    (key: string) => liftedKeys(order, key, selectedKeys),
    [order, selectedKeys],
  );
  return (
    <CardDragLayer
      order={order}
      liftedFor={liftedFor}
      reorder={false}
      onto={drops.onto}
      nameOf={nameOf}
      preview={preview}
      onDrop={drops.hold}
      onLift={onLift}
      instructions={t.organize.instructions}
      held={drops.held}
      attached={drops.attached}
    >
      <CardGrid size={zoom}>
        {shown.flatMap((item) => {
          const video = itemVideo(item);
          if (video === undefined) return [];
          return [
            <DraggableCard key={itemKey(item)} id={itemKey(item)}>
              {card(video)}
            </DraggableCard>,
          ];
        })}
      </CardGrid>
    </CardDragLayer>
  );
}
