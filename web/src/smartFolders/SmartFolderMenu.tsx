import { Ellipsis, Pencil, Plus, Trash2 } from "lucide-react";
import { type Ref, useState } from "react";

import type { SmartFolderSettings, SmartFolderSummary } from "../api/smartFolders";
import { t } from "../i18n";
import { Button } from "../ui/shadcn/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "../ui/shadcn/dropdown-menu";
import { Tooltip, TooltipContent, TooltipTrigger } from "../ui/shadcn/tooltip";
import { useToast } from "../ui/Toast";
import { DEFAULT_SORT } from "../videoList/listCriteria";
import DeleteSmartFolderDialog from "./DeleteSmartFolderDialog";
import SmartFolderDialog from "./SmartFolderDialog";

type Dialog = "edit" | "newChild" | "delete" | null;

/** SmartFolderMenuProps はフォルダのページの「More」の props である。 */
export interface SmartFolderMenuProps {
  folder: { id: number; name: string; parentId?: number };
  /** 所有者だけが受け取る条件・並べ順・もう無い参照。 */
  settings: SmartFolderSettings;
  moreRef?: Ref<HTMLButtonElement>;
  /** 直せた（名前・親・並べ順・条件）。呼び手が読み直す。 */
  onSaved: (folder: SmartFolderSummary) => void;
  /** 新しい子を作れた。呼び手が読み直す。 */
  onCreatedChild: (folder: SmartFolderSummary) => void;
  /** 消えた。`announce` が偽なら、もう無かった（トーストを出さない）。 */
  onDeleted: (announce: boolean) => void;
  /** 直そうとしたらフォルダ自身がもう無かった。 */
  onGone: () => void;
}

/**
 * SmartFolderMenu は所有者のスマートフォルダのページの見出しの「More」である
 * （specs/052-smart-folders/ui-design.md「Header」）。「Edit…」「New child smart folder…」、区切り、
 * 「Delete…」を開き、それぞれの窓を持つ。直せたら `Saved "{name}"`、子を作れたら
 * `Created "{name}"` のトーストを出す。消えた後の移動とトーストは呼び手が持つ。
 */
export default function SmartFolderMenu({
  folder,
  settings,
  moreRef,
  onSaved,
  onCreatedChild,
  onDeleted,
  onGone,
}: SmartFolderMenuProps) {
  const toast = useToast();
  const [dialog, setDialog] = useState<Dialog>(null);
  const close = () => setDialog(null);

  return (
    <>
      <DropdownMenu>
        <Tooltip>
          <TooltipTrigger asChild>
            <DropdownMenuTrigger asChild>
              <Button
                ref={moreRef}
                variant="ghost"
                size="icon-sm"
                aria-label={t.common.more}
              >
                <Ellipsis aria-hidden="true" />
              </Button>
            </DropdownMenuTrigger>
          </TooltipTrigger>
          <TooltipContent>{t.common.more}</TooltipContent>
        </Tooltip>
        <DropdownMenuContent align="end">
          <DropdownMenuItem onSelect={() => setDialog("edit")}>
            <Pencil aria-hidden="true" />
            {t.smartFolders.detail.edit}
          </DropdownMenuItem>
          <DropdownMenuItem onSelect={() => setDialog("newChild")}>
            <Plus aria-hidden="true" />
            {t.smartFolders.detail.newChild}
          </DropdownMenuItem>
          <DropdownMenuSeparator />
          <DropdownMenuItem variant="destructive" onSelect={() => setDialog("delete")}>
            <Trash2 aria-hidden="true" />
            {t.smartFolders.detail.delete}
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
      {dialog === "edit" && (
        <SmartFolderDialog
          target={{
            kind: "edit",
            folder,
            rule: settings.rule,
            sort: settings.sort,
            missing: settings.missing,
          }}
          onClose={close}
          onDone={(saved) => {
            close();
            toast(t.smartFolders.saved(saved.name));
            onSaved(saved);
          }}
          onGone={() => {
            close();
            onGone();
          }}
        />
      )}
      {dialog === "newChild" && (
        <SmartFolderDialog
          target={{
            kind: "create",
            rule: {},
            sort: DEFAULT_SORT,
            parent: { id: folder.id, name: folder.name },
          }}
          onClose={close}
          onDone={(created) => {
            close();
            toast(t.smartFolders.created(created.name));
            onCreatedChild(created);
          }}
        />
      )}
      {dialog === "delete" && (
        <DeleteSmartFolderDialog
          folder={folder}
          onClose={close}
          onDeleted={(announce) => {
            close();
            onDeleted(announce);
          }}
        />
      )}
    </>
  );
}
