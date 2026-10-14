import { type FolderRef, getGroup, RequestFailed } from "../api/client";
import { folderRefKey, groupFolder } from "../api/libraryItems";

/** 同時に読むグループの数。「すべて選択」のグループが多くても要求を並べすぎない。 */
const concurrency = 6;

/**
 * withGroupFolders は `folders` に、「すべて選択」で読み込んでいないグループ `groupIds` の
 * フォルダを `GET /api/groups/{id}` で引いて足す。お気に入りとプレイリストはまだフォルダで送り
 * （specs/048-groups/plan.md「Key favorites and playlist items by group id」までのつなぎ）、
 * 「すべて選択」の `groups` はフォルダを持たない（specs/048-groups/contracts/screen-api.md）。
 * フォルダの無い（手で作った）グループと、もう無いか見せない（404）グループは足さない。ほかの
 * 失敗はそのまま投げる。
 */
export async function withGroupFolders(
  folders: readonly FolderRef[],
  groupIds: readonly number[],
): Promise<FolderRef[]> {
  if (groupIds.length === 0) return [...folders];
  const found: (FolderRef | undefined)[] = [];
  const queue = groupIds.entries();
  const worker = async () => {
    for (const [index, id] of queue) found[index] = await folderOf(id);
  };
  await Promise.all(
    Array.from({ length: Math.min(concurrency, groupIds.length) }, () => worker()),
  );
  const out = [...folders];
  const seen = new Set(folders.map(folderRefKey));
  for (const folder of found) {
    if (folder === undefined || seen.has(folderRefKey(folder))) continue;
    seen.add(folderRefKey(folder));
    out.push(folder);
  }
  return out;
}

async function folderOf(id: number): Promise<FolderRef | undefined> {
  try {
    return groupFolder((await getGroup(id)).group);
  } catch (failure) {
    if (failure instanceof RequestFailed && failure.status === 404) return undefined;
    throw failure;
  }
}
