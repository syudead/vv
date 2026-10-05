import { ChevronDown, Group, LoaderCircle, Tag, Ungroup } from "lucide-react";
import { useState } from "react";

import type { FolderRef } from "../api/client";
import {
  type FolderGrouping,
  type FolderGroupingMode,
  groupingFailure,
  setFolderGrouping,
  taggedMessage,
  tagFolderGroup,
} from "../api/folderGrouping";
import { t } from "../i18n";
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
 * タグ化の成功は `onTagged` で、画面に動画の一覧を読み直させる。409 は `onConflict` で
 * フォルダの一覧を取り直させる。どちらの操作も、成功すると一覧の控えを捨てる（api/folderGrouping）。
 */
export default function FolderGroupingMenu({
  folder,
  name,
  grouping,
  onGroupingChange,
  onTagged,
  onConflict,
}: {
  folder: FolderRef;
  /** フォルダの名前（失敗の文言に使う）。 */
  name: string;
  grouping: FolderGrouping;
  onGroupingChange: (grouping: FolderGrouping) => void;
  onTagged: () => void;
  onConflict: () => void;
}) {
  const toast = useToast();
  const [busy, setBusy] = useState(false);

  const fail = (failure: unknown, tagging: boolean) => {
    const { message, conflict } = groupingFailure(failure, {
      name,
      tagging,
      notFoundMessage: t.folders.notFound.title,
    });
    toast(message);
    if (conflict) onConflict();
  };

  // 同じ値を選び直しても送る（ui-design.md「Folder grouping menu」。応答は 200）。
  const choose = async (mode: FolderGroupingMode) => {
    setBusy(true);
    try {
      onGroupingChange(await setFolderGrouping(folder, mode));
    } catch (failure) {
      fail(failure, false);
    } finally {
      setBusy(false);
    }
  };

  const toTag = async () => {
    setBusy(true);
    try {
      const result = await tagFolderGroup(folder);
      toast(taggedMessage(result));
      onGroupingChange(result.grouping);
      onTagged();
    } catch (failure) {
      fail(failure, true);
    } finally {
      setBusy(false);
    }
  };

  const Icon = busy ? LoaderCircle : grouping.grouped ? Group : Ungroup;
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          variant="ghost"
          size="sm"
          disabled={busy}
          aria-busy={busy}
          aria-label={
            busy
              ? t.folders.grouping.changing
              : t.folders.grouping.trigger(grouping.grouped)
          }
          // 見出しと同じ弱さにし、見出しの行の高さを変えない（ui-design.md「Visual review criteria」）。
          className="-my-2 -mr-2 gap-1.5 text-muted-foreground"
        >
          <Icon
            aria-hidden="true"
            className={busy ? "animate-spin motion-reduce:animate-none" : undefined}
          />
          {busy
            ? t.folders.grouping.changingShort
            : grouping.grouped
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
        {grouping.taggable && (
          <>
            <DropdownMenuSeparator />
            <DropdownMenuItem onSelect={() => void toTag()}>
              <Tag aria-hidden="true" />
              {t.folders.grouping.toTag}
            </DropdownMenuItem>
          </>
        )}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
