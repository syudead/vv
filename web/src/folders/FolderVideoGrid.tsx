import { type ReactNode, useMemo } from "react";

import type { Video } from "../api/client";
import { isOrganizable, itemKey, itemVideo, itemVideos } from "../api/libraryItems";
import type { VideosState } from "../api/useVideos";
import { useAudience } from "../auth/audience";
import { t } from "../i18n";
import type { Zoom } from "../preferences/viewPreferences";
import { CardGrid } from "../ui/patterns/card-grid";
import { CardDragLayer, DraggableCard } from "../videoList/cardDrag";
import { useOrderedItemDrag } from "../videoList/OrderedItemFace";
import { useUndoToast } from "../videoList/undoToast";
import { useVideoDrops } from "../videoList/useVideoDrops";

/** single はフォルダ画面の持ち上げ方である。選択が無いので、押したカード 1 枚だけが動く。 */
function single(key: string): readonly string[] {
  return [key];
}

/**
 * FolderVideoGrid はフォルダ画面の動画のカードの格子である。所有者には動画のカードを 1 枚ずつ
 * ドラッグさせ、動画のカードだけを落とす先にする（specs/050-drag-organize/ui-design.md
 * 「Folder screen」）。フォルダの直下と検索結果が共有する。
 */
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
  /** 動画 1 本のカード。 */
  card: (video: Video) => ReactNode;
  /** 持ち上がったときに呼ぶ（ホバープレビューを止める）。 */
  onLift: () => void;
}

function OwnerFolderVideoGrid({ videos, zoom, card, onLift }: FolderVideoGridProps) {
  const { refreshInPlace } = videos;
  const undo = useUndoToast(refreshInPlace);
  const drops = useVideoDrops({
    items: videos.items,
    keyOf: itemKey,
    videoOf: itemVideo,
    from: null,
    groupItem: null,
    reread: refreshInPlace,
    undo,
  });
  const shown = drops.shown;
  // フォルダ画面にスマートフォルダの項目は来ないが、型の上で持ち上げる項目に絞る。
  const organizable = useMemo(() => shown.filter(isOrganizable), [shown]);
  const { order, nameOf, preview } = useOrderedItemDrag(organizable, itemKey, false);
  return (
    <CardDragLayer
      order={order}
      liftedFor={single}
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
