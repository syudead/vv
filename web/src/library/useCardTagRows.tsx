import { useCallback } from "react";

import type { LibraryGroup, TagRef, Video, VideoTag } from "../api/client";
import CardTagRow from "./CardTagRow";

/** ignoreToggle は選択を持たないカードのタグの行の、選択を切り替えない口である。 */
const ignoreToggle = () => undefined;

/**
 * useFolderTagRow はスマートフォルダのカードのタグの行を作る関数である（SmartFolderCard の
 * tagsRow）。フォルダのカードは選択を持たないので、ほかのカードを選んでいる間もチップは押せる
 * ままで、押すとそのタグで絞り込む。ライブラリとスマートフォルダのページが共有する。
 */
export function useFolderTagRow(onPressTag: (tag: TagRef) => void) {
  return useCallback(
    (tags: readonly VideoTag[]) => (
      <CardTagRow
        tags={tags}
        selectionMode={false}
        onPress={onPressTag}
        onToggleSelection={ignoreToggle}
      />
    ),
    [onPressTag],
  );
}

/**
 * useCardTagRows はカードの題名の下のタグの行を作る関数である（VideoCard・GroupCard・
 * SmartFolderCard の tagsRow）。チップを押すと `onPressTag`、選択中に押すと動画とグループの
 * カードの選択を切り替える。スマートフォルダのカードは選択を持たない（useFolderTagRow）。グループのページと
 * スマートフォルダのページが共有する。
 */
export function useCardTagRows({
  selectionMode,
  onPressTag,
  toggleVideo,
  toggleGroup,
}: {
  selectionMode: boolean;
  onPressTag: (tag: TagRef) => void;
  toggleVideo: (videoId: number) => void;
  toggleGroup: (group: LibraryGroup) => void;
}) {
  const videoTags = useCallback(
    (video: Video) => (
      <CardTagRow
        tags={video.tags}
        selectionMode={selectionMode}
        onPress={onPressTag}
        onToggleSelection={() => toggleVideo(video.id)}
      />
    ),
    [onPressTag, selectionMode, toggleVideo],
  );
  const groupTags = useCallback(
    (group: LibraryGroup) => (
      <CardTagRow
        tags={group.tags}
        selectionMode={selectionMode}
        onPress={onPressTag}
        onToggleSelection={() => toggleGroup(group)}
      />
    ),
    [onPressTag, selectionMode, toggleGroup],
  );
  const folderTags = useFolderTagRow(onPressTag);
  return { videoTags, groupTags, folderTags };
}
