import { ChevronDown, Group, LoaderCircle, Tag, Ungroup } from "lucide-react";
import { useState } from "react";

import { type FolderRef, getGroup, RequestFailed } from "../api/client";
import {
  type FolderGrouping,
  type FolderGroupingMode,
  groupingFailure,
  setFolderGrouping,
} from "../api/folderGrouping";
import { taggedMessage } from "../api/groups";
import { clearListSnapshot } from "../api/listSnapshot";
import GroupTagDialog from "../groups/GroupTagDialog";
import { errorText, t } from "../i18n";
import { Button } from "../ui/shadcn/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "../ui/shadcn/dropdown-menu";
import { useToast } from "../ui/Toast";

/** modes はまとめ方の選択肢の並びである。表示名は描画のたびにカタログから引く。 */
const modes: readonly FolderGroupingMode[] = ["auto", "ungroup", "groupDirect"];

/**
 * FolderGroupingMenu はフォルダ画面の「動画 N」の見出しの行の右端に置く、そのフォルダの
 * ライブラリでのまとめ方のメニューである（specs/017-folder-groups/ui-design.md
 * 「Folder grouping menu」、要件 7・11・22）。
 *
 * ラジオは応答の `grouping` を `onGroupingChange` で返すだけで、一覧は読み直させない。
 * 「Turn the group into a tag…」は `grouping.groupId`（そのフォルダの自動グループ）があるときだけ
 * 描き、共有のタグの窓（GroupTagDialog）を開く（specs/048-groups/ui-design.md「Folder grouping
 * menu」）。グループは改名できるので、窓の名前は開く前に `GET /api/groups/{id}` で読んだ今の名前に
 * する（タグはその名前で作られる）。その読み出しの 404 は窓からの 404 と同じに扱う。成功は `onTagged` で画面に動画の一覧を読み直させ、窓からの 404（途中で解散された）と
 * ラジオの 409 は `onStale` でフォルダの `grouping` を読み直させる。どちらの操作も、成功すると
 * 一覧の控えを捨てる（api/folderGrouping、api/groups）。
 */
export default function FolderGroupingMenu({
  folder,
  name,
  grouping,
  onGroupingChange,
  onTagged,
  onStale,
}: {
  folder: FolderRef;
  /** フォルダの名前（失敗の文言に使う）。 */
  name: string;
  grouping: FolderGrouping;
  onGroupingChange: (grouping: FolderGrouping) => void;
  onTagged: () => void;
  onStale: () => void;
}) {
  const toast = useToast();
  const [busy, setBusy] = useState(false);
  const [tagging, setTagging] = useState<{ id: number; name: string } | null>(null);

  const fail = (failure: unknown) => {
    const { message, conflict } = groupingFailure(failure, {
      name,
      tagging: false,
      notFoundMessage: t.folders.notFound.title,
    });
    toast(message);
    if (conflict) onStale();
  };

  // 同じ値を選び直しても送る（ui-design.md「Folder grouping menu」。応答は 200）。
  const choose = async (mode: FolderGroupingMode) => {
    setBusy(true);
    try {
      onGroupingChange(await setFolderGrouping(folder, mode));
    } catch (failure) {
      fail(failure);
    } finally {
      setBusy(false);
    }
  };

  // グループが途中で解散された。ライブラリの控えにはまだそのカードがあるので捨てる。
  const gone = () => {
    setTagging(null);
    clearListSnapshot();
    onStale();
  };

  const openTagDialog = async (id: number) => {
    try {
      const detail = await getGroup(id);
      setTagging({ id, name: detail.group.name });
    } catch (failure) {
      if (failure instanceof RequestFailed && failure.status === 404) gone();
      else toast(t.groups.tagDialog.failed(errorText(failure)));
    }
  };

  const groupId = grouping.groupId;
  const grouped = groupId !== undefined;
  const Icon = busy ? LoaderCircle : grouped ? Group : Ungroup;
  return (
    <>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button
            variant="ghost"
            size="sm"
            disabled={busy}
            aria-busy={busy}
            aria-label={
              busy ? t.folders.grouping.changing : t.folders.grouping.trigger(grouped)
            }
          >
            <Icon
              aria-hidden="true"
              className={busy ? "animate-spin motion-reduce:animate-none" : undefined}
            />
            {busy
              ? t.folders.grouping.changingShort
              : grouped
                ? t.folders.grouping.grouped
                : t.folders.grouping.ungrouped}
            <ChevronDown aria-hidden="true" />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end" className="max-w-popover-wide">
          <DropdownMenuLabel>{t.folders.grouping.heading}</DropdownMenuLabel>
          <DropdownMenuRadioGroup
            value={grouping.mode}
            onValueChange={(mode) => void choose(mode as FolderGroupingMode)}
          >
            {modes.map((mode) => (
              <DropdownMenuRadioItem key={mode} value={mode}>
                {t.folders.grouping.modes[mode]}
              </DropdownMenuRadioItem>
            ))}
          </DropdownMenuRadioGroup>
          <p className="px-2 pt-1 pb-1.5 text-xs text-muted-foreground">
            {t.folders.grouping.autoHint}
          </p>
          {grouped && (
            <>
              <DropdownMenuSeparator />
              <DropdownMenuItem
                onSelect={() => {
                  if (groupId !== undefined) void openTagDialog(groupId);
                }}
              >
                <Tag aria-hidden="true" />
                {t.folders.grouping.toTag}
              </DropdownMenuItem>
            </>
          )}
        </DropdownMenuContent>
      </DropdownMenu>
      {tagging !== null && (
        <GroupTagDialog
          group={tagging}
          onClose={() => setTagging(null)}
          onTagged={(result) => {
            setTagging(null);
            toast(taggedMessage(result));
            // タグにすると自動グループは解散し、そのフォルダに ungroup が書かれる（research.md R-4・R-11）。
            onGroupingChange({ mode: "ungroup" });
            onTagged();
          }}
          onGone={gone}
        />
      )}
    </>
  );
}
