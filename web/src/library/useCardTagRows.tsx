import { useCallback } from "react";

import type { LibraryGroup, TagRef, Video, VideoTag } from "../api/client";
import CardTagRow from "./CardTagRow";

const ignoreToggle = () => undefined;

// Without a toggle a smart folder cannot be selected, so its chips remain interactive.
export function useFolderTagRow(
  onPressTag: (tag: TagRef) => void,
  selectionMode = false,
  toggleFolder?: (folderId: number) => void,
) {
  return useCallback(
    (tags: readonly VideoTag[], folderId: number) => (
      <CardTagRow
        tags={tags}
        selectionMode={selectionMode && toggleFolder !== undefined}
        onPress={onPressTag}
        onToggleSelection={
          toggleFolder === undefined ? ignoreToggle : () => toggleFolder(folderId)
        }
      />
    ),
    [onPressTag, selectionMode, toggleFolder],
  );
}

// In selection mode, card chips toggle selection.
export function useCardTagRows({
  selectionMode,
  onPressTag,
  toggleVideo,
  toggleGroup,
  toggleSmartFolder,
}: {
  selectionMode: boolean;
  onPressTag: (tag: TagRef) => void;
  toggleVideo: (videoId: number) => void;
  toggleGroup: (group: LibraryGroup) => void;
  toggleSmartFolder?: (folderId: number) => void;
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
  const folderTags = useFolderTagRow(onPressTag, selectionMode, toggleSmartFolder);
  return { videoTags, groupTags, folderTags };
}
