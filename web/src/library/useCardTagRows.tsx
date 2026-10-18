import { useCallback } from "react";

import type { LibraryGroup, TagRef, Video, VideoTag } from "../api/client";
import CardTagRow from "./CardTagRow";

const ignoreToggle = () => undefined;

// Smart folders have no selection mode, so their chips remain interactive.
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

// In selection mode, ordinary card chips toggle selection; smart folder chips stay interactive.
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
