import { useCallback } from "react";
import { useNavigate } from "react-router";

import type { TagRef, Video } from "../api/client";
import CardTagRow from "../library/CardTagRow";

const noop = () => undefined;

/** Folder tags link to the filtered library; stabilize the callback for memoized VideoCard. */
export function useFolderTagsRow(): (video: Video) => React.ReactNode {
  const navigate = useNavigate();
  const pressTag = useCallback(
    (tag: TagRef) => void navigate(`/?tag=${String(tag.id)}`),
    [navigate],
  );
  return useCallback(
    (video: Video) => (
      <CardTagRow
        tags={video.tags}
        selectionMode={false}
        onPress={pressTag}
        onToggleSelection={noop}
      />
    ),
    [pressTag],
  );
}
