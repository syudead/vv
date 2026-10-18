import { Ellipsis, House, Minus, Pencil, Plus, Trash2 } from "lucide-react";
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

export type SmartFolderMenuDialog = "edit" | "newChild" | "delete" | null;

export interface SmartFolderMenuProps {
  folder: { id: number; name: string; parentId?: number };
  /** Owner-only settings and missing references. */
  settings: SmartFolderSettings;
  moreRef?: Ref<HTMLButtonElement>;
  onSaved: (folder: SmartFolderSummary) => void;
  onCreatedChild: (folder: SmartFolderSummary) => void;
  /** False means already missing; suppress the toast. */
  onDeleted: (announce: boolean) => void;
  onGone: () => void;
  /** Optional controlled dialog state lets the empty-state action share this dialog. */
  dialog?: SmartFolderMenuDialog;
  onDialogChange?: (dialog: SmartFolderMenuDialog) => void;
  /**
   * フォルダがホームの棚か（応答の `onHome`）と、その切り替え。渡すと「Edit…」の後に
   * 「Add to home」か「Remove from home」を出す（docs/design-docs/home.md「Smart folder page
   * and Settings」）。
   */
  home?: { onHome: boolean; busy: boolean; toggle: () => void };
}

/** The caller owns post-deletion navigation and notification. */
export default function SmartFolderMenu({
  folder,
  settings,
  moreRef,
  onSaved,
  onCreatedChild,
  onDeleted,
  onGone,
  home,
  ...control
}: SmartFolderMenuProps) {
  const toast = useToast();
  const [ownDialog, setOwnDialog] = useState<SmartFolderMenuDialog>(null);
  const dialog = control.dialog === undefined ? ownDialog : control.dialog;
  const setDialog = control.onDialogChange ?? setOwnDialog;
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
          {home !== undefined && (
            <DropdownMenuItem disabled={home.busy} onSelect={home.toggle}>
              {home.onHome ? <Minus aria-hidden="true" /> : <House aria-hidden="true" />}
              {home.onHome ? t.home.removeFromHome : t.home.addToHome}
            </DropdownMenuItem>
          )}
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
