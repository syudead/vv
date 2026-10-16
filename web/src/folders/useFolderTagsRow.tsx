import { useCallback } from "react";
import { useNavigate } from "react-router";

import type { TagRef, Video } from "../api/client";
import CardTagRow from "../library/CardTagRow";
import { libraryFilterPath } from "../library/libraryPath";
import { TAG_PARAM } from "../library/tagCriteria";

const noop = () => undefined;

/**
 * useFolderTagsRow はフォルダ画面のカードへ渡すタグの行である（VideoCard の
 * `tagsRow`）。フォルダ画面は選択を持たないので、チップを押すと再生画面のタグと
 * 同じく、そのタグで絞ったライブラリ（`/library?tag=<id>`）へ移る。参照を保つため
 * useCallback で包む（memo(VideoCard) を効かせる）。
 */
export function useFolderTagsRow(): (video: Video) => React.ReactNode {
  const navigate = useNavigate();
  const pressTag = useCallback(
    (tag: TagRef) => void navigate(libraryFilterPath(TAG_PARAM, tag.id)),
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
