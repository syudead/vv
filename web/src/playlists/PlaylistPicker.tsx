import { ListVideo } from "lucide-react";
import { useEffect, useRef } from "react";

import { RequestFailed } from "../api/client";
import {
  addToPlaylist,
  createPlaylist,
  listAllPlaylists,
  type PlaylistSummary,
} from "../api/playlists";
import { t } from "../i18n";
import NamedPicker, { type PickerChoice } from "../library/NamedPicker";
import { resultText } from "./pickerRules";

const noSmartFolders: readonly number[] = [];

/** Reuse a playlist already created under this name in the current memory map. */
async function createOnce(
  memory: Map<string, PlaylistSummary>,
  name: string,
): Promise<PlaylistSummary> {
  const earlier = memory.get(name);
  if (earlier !== undefined) return earlier;
  const playlist = await createPlaylist(name);
  memory.set(name, playlist);
  return playlist;
}

/**
 * Reload all playlists on opening; remember newly created playlists so failed additions retry
 * without creating duplicates. Forget a missing playlist so the next attempt can recreate it.
 */
export default function PlaylistPicker({
  open,
  onOpenChange,
  videoIds,
  groupIds,
  smartFolderIds = noSmartFolders,
  side = "bottom",
  align = "end",
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  videoIds: readonly number[];
  groupIds: readonly number[];
  smartFolderIds?: readonly number[];
  side?: "top" | "bottom";
  align?: "start" | "end";
}) {
  // Use a fresh name-to-playlist map on each opening; in-flight requests write only to their original map. Reusing created playlists avoids leaving duplicates empty after failed additions.
  const created = useRef(new Map<string, PlaylistSummary>());
  useEffect(() => {
    if (open) created.current = new Map();
  }, [open]);

  // Capture this render's selection so pending submissions do not change with later props.
  async function submit(choice: PickerChoice<PlaylistSummary>) {
    const memory = created.current;
    let playlist: { id: number; name: string } | null = null;
    try {
      playlist =
        "target" in choice ? choice.target : await createOnce(memory, choice.create);
      const result = await addToPlaylist(
        playlist.id,
        videoIds,
        groupIds,
        undefined,
        smartFolderIds,
      );
      return resultText(result, playlist.name);
    } catch (error: unknown) {
      // Forget a playlist deleted in another tab so creation can retry.
      if (error instanceof RequestFailed && error.status === 404 && playlist !== null) {
        const gone = playlist.id;
        for (const [name, kept] of memory) {
          if (kept.id === gone) memory.delete(name);
        }
      }
      throw error;
    }
  }

  return (
    <NamedPicker
      open={open}
      onOpenChange={onOpenChange}
      words={t.playlists.add}
      icon={ListVideo}
      memberCount={videoIds.length + groupIds.length + smartFolderIds.length}
      loadAll={listAllPlaylists}
      submit={submit}
      side={side}
      align={align}
    />
  );
}
