import { FolderSearch, Plus } from "lucide-react";
import { useEffect, useMemo, useRef, useState } from "react";
import { useNavigate } from "react-router";

import { LibraryTableHeader } from "../library/LibraryPage";
import { t } from "../i18n";
import { DataTable } from "../ui/patterns/data-table";
import { EmptyState } from "../ui/patterns/empty-state";
import { ErrorState } from "../ui/patterns/error-state";
import { ListPage } from "../ui/patterns/list-page";
import { LoadingState } from "../ui/patterns/loading-state";
import { PageHeader } from "../ui/patterns/page-header";
import { Button } from "../ui/shadcn/button";
import { TableBody } from "../ui/shadcn/table";
import { useToast } from "../ui/Toast";
import { DEFAULT_SORT } from "../videoList/listCriteria";
import { SmartFolderCardsProvider } from "./cardValues";
import { SmartFolderRow } from "./SmartFolderCard";
import SmartFolderDialog from "./SmartFolderDialog";
import { smartFolderPagePath } from "./smartFolderPaths";
import { useSmartFolderList } from "./useSmartFolderList";

const listPath = "/smart-folders";

/**
 * The owner's root smart folders in the server's name order. A creation reads the list again
 * instead of inserting locally, since the server's name order folds width, case and kana. A child
 * folder has no row here, so the page opens the new folder's page instead.
 */
export default function SmartFolderListPage() {
  const { state, read, reload } = useSmartFolderList(true);
  const [creating, setCreating] = useState(false);
  const [focusId, setFocusId] = useState<number | null>(null);
  const table = useRef<HTMLDivElement | null>(null);
  const toast = useToast();
  const navigate = useNavigate();

  useEffect(() => {
    const previous = document.title;
    document.title = t.smartFolders.list.documentTitle;
    return () => {
      document.title = previous;
    };
  }, []);

  const roots = useMemo(
    () =>
      state.kind === "content"
        ? state.folders.filter((folder) => folder.parentId === undefined)
        : [],
    [state],
  );

  // Focus the created folder once a read shows its row, after the dialog restores its trigger's focus.
  useEffect(() => {
    if (focusId === null) return;
    if (state.kind === "failed") {
      setFocusId(null);
      return;
    }
    const link = table.current?.querySelector<HTMLAnchorElement>(
      `[data-smart-folder-id="${String(focusId)}"] a`,
    );
    if (link === null || link === undefined) return;
    const timer = setTimeout(() => {
      link.focus();
      setFocusId(null);
    }, 0);
    return () => clearTimeout(timer);
  }, [focusId, state]);

  const empty = state.kind === "content" && roots.length === 0;
  const newButton = (
    <Button size="sm" onClick={() => setCreating(true)}>
      <Plus aria-hidden="true" />
      {t.smartFolders.list.new}
    </Button>
  );

  return (
    <ListPage
      header={
        <PageHeader
          title={t.smartFolders.list.title}
          wrapTitle
          count={
            state.kind === "content" && !empty
              ? t.smartFolders.list.count(roots.length)
              : undefined
          }
          // In the empty state the state block holds the one creation action.
          actions={empty ? undefined : newButton}
        />
      }
    >
      {state.kind === "loading" && (
        <LoadingState label={t.list.loading} layout="table" count={6} />
      )}
      {state.kind === "failed" && (
        <ErrorState
          title={t.smartFolders.listFailed(state.reason)}
          retryLabel={t.common.retry}
          onRetry={reload}
        />
      )}
      {empty && (
        <EmptyState
          icon={<FolderSearch aria-hidden="true" />}
          title={t.smartFolders.list.empty.title}
          description={t.smartFolders.list.empty.description}
          action={newButton}
        />
      )}
      {roots.length > 0 && (
        <div ref={table}>
          <SmartFolderCardsProvider>
            <DataTable label={t.smartFolders.list.table}>
              <LibraryTableHeader owner folderList />
              <TableBody>
                {roots.map((folder) => (
                  <SmartFolderRow
                    key={folder.id}
                    folder={folder}
                    backTo={listPath}
                    owner
                    folderList
                  />
                ))}
              </TableBody>
            </DataTable>
          </SmartFolderCardsProvider>
        </div>
      )}
      {creating && (
        <SmartFolderDialog
          target={{ kind: "create", rule: {}, sort: DEFAULT_SORT }}
          onClose={() => setCreating(false)}
          onDone={(created) => {
            setCreating(false);
            toast(t.smartFolders.created(created.name));
            if (created.parentId !== undefined) {
              void navigate(smartFolderPagePath(created.id), {
                state: { from: listPath },
              });
              return;
            }
            setFocusId(created.id);
            read();
          }}
        />
      )}
    </ListPage>
  );
}
