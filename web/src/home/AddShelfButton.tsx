import { FolderSearch, Plus } from "lucide-react";
import { useEffect, useState } from "react";

import { isAborted } from "../api/client";
import { listSmartFolders, type SmartFolderSummary } from "../api/smartFolders";
import { errorText, t, type UiText } from "../i18n";
import { Button } from "../ui/shadcn/button";
import {
  Command,
  CommandEmpty,
  CommandInput,
  CommandItem,
  CommandList,
} from "../ui/shadcn/command";
import { Popover, PopoverContent, PopoverTrigger } from "../ui/shadcn/popover";
import { Spinner } from "../ui/shadcn/spinner";
import type { ShelfFolder } from "./useShelfWrites";

type Folders =
  | { status: "loading" }
  | { status: "failed"; reason: UiText }
  | { status: "ready"; folders: SmartFolderSummary[] };

/**
 * AddShelfButton はホームの見出しの「Add shelf」である（docs/design-docs/home.md「Home」）。
 * 開くたびにスマートフォルダを読み、ホームに無いものを名前で絞って選ぶ一覧を出す。選ぶと
 * `onPick` を呼んで閉じる。足す先（最後のフォルダの棚の後）は呼び手が決める。
 */
export default function AddShelfButton({
  onHome,
  disabled,
  onPick,
}: {
  /** もうホームにあるフォルダの id。 */
  onHome: readonly number[];
  disabled: boolean;
  onPick: (folder: ShelfFolder) => void;
}) {
  const [open, setOpen] = useState(false);
  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button variant="outline" size="sm" disabled={disabled}>
          <Plus aria-hidden="true" />
          {t.home.addShelf}
        </Button>
      </PopoverTrigger>
      {open && (
        <PopoverContent align="end" className="p-0">
          <ShelfPicker
            onHome={onHome}
            onPick={(folder) => {
              setOpen(false);
              onPick(folder);
            }}
          />
        </PopoverContent>
      )}
    </Popover>
  );
}

/** ShelfPicker は開いた「Add shelf」の中身である。読んでいる間・失敗・候補の無いときの文も出す。 */
function ShelfPicker({
  onHome,
  onPick,
}: {
  onHome: readonly number[];
  onPick: (folder: ShelfFolder) => void;
}) {
  const [folders, setFolders] = useState<Folders>({ status: "loading" });
  useEffect(() => {
    const controller = new AbortController();
    listSmartFolders(controller.signal).then(
      (list) => setFolders({ status: "ready", folders: list }),
      (error: unknown) => {
        if (!isAborted(error)) setFolders({ status: "failed", reason: errorText(error) });
      },
    );
    return () => controller.abort();
  }, []);

  if (folders.status === "loading") {
    return (
      <div className="flex justify-center p-3">
        <Spinner />
      </div>
    );
  }
  const note = pickerNote(folders, onHome);
  if (note !== null) {
    return <p className="p-3 text-sm text-muted-foreground">{note}</p>;
  }
  const choices = (folders.status === "ready" ? folders.folders : []).filter(
    (folder) => !onHome.includes(folder.id),
  );
  return (
    <Command label={t.home.addShelf}>
      <CommandInput
        placeholder={t.home.addShelfSearch}
        aria-label={t.home.addShelfSearch}
      />
      <CommandList label={t.home.addShelfList}>
        <CommandEmpty>{t.home.addShelfNoMatch}</CommandEmpty>
        {choices.map((folder) => (
          <CommandItem
            key={folder.id}
            value={String(folder.id)}
            keywords={[folder.name]}
            onSelect={() => onPick({ id: folder.id, name: folder.name })}
          >
            <FolderSearch aria-hidden="true" />
            <span className="truncate">{folder.name}</span>
          </CommandItem>
        ))}
      </CommandList>
    </Command>
  );
}

/**
 * pickerNote は一覧の代わりに出す文である。読めなかった、スマートフォルダが 1 つも無い、
 * どれももうホームにある、のどれでもなければ null（ui-design.md「Words」）。
 */
function pickerNote(folders: Folders, onHome: readonly number[]): UiText | null {
  if (folders.status === "failed") return t.home.addShelfLoadFailed(folders.reason);
  if (folders.status !== "ready") return null;
  if (folders.folders.length === 0) return t.home.addShelfNone;
  return folders.folders.every((folder) => onHome.includes(folder.id))
    ? t.home.addShelfAllOnHome
    : null;
}
