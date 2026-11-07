import { ChevronRight } from "lucide-react";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Link } from "react-router";

import type { LibraryItem, SmartFolderSummary } from "../api/client";
import { subscribeServerEvents } from "../api/serverEvents";
import { listSmartFolders } from "../api/smartFolders";
import { errorText, t } from "../i18n";
import { SmartFolderCardsProvider } from "../smartFolders/cardValues";
import {
  SmartFolderCard,
  type SmartFolderCardProps,
} from "../smartFolders/SmartFolderCard";
import { ErrorState } from "../ui/patterns/error-state";
import { Shelf } from "../ui/patterns/shelf";
import { Button } from "../ui/shadcn/button";
import { Separator } from "../ui/shadcn/separator";

/** Nothing is drawn until the list arrives, so a band that turns out absent never flashes. */
export type FavoriteFolders =
  | { kind: "none" }
  | { kind: "content"; folders: readonly SmartFolderSummary[] }
  | { kind: "failed"; reason: string };

const none: FavoriteFolders = { kind: "none" };
const noItems: readonly LibraryItem[] = [];

/**
 * The owner's favorite smart folders at any depth, in the list's name order. The whole list is
 * read again on every `smartFolder` and `peerItems` event and on reconnection, since a favorite
 * change names every folder and a link that connects or drops changes the merged list.
 */
export function useFavoriteFolders(enabled: boolean) {
  const [state, setState] = useState<FavoriteFolders>(none);
  const current = useRef<AbortController | null>(null);

  const read = useCallback(() => {
    current.current?.abort();
    const controller = new AbortController();
    current.current = controller;
    listSmartFolders(controller.signal).then(
      (folders) => {
        if (controller.signal.aborted) return;
        setState({
          kind: "content",
          folders: folders.filter((folder) => folder.favorite === true),
        });
      },
      (error: unknown) => {
        if (controller.signal.aborted) return;
        setState({ kind: "failed", reason: errorText(error) });
      },
    );
  }, []);

  useEffect(() => {
    if (!enabled) {
      setState(none);
      return;
    }
    read();
    const unsubscribe = subscribeServerEvents({
      smartFolder: read,
      peerItems: read,
      open: (reconnected) => {
        if (reconnected) read();
      },
    });
    return () => {
      unsubscribe();
      current.current?.abort();
      current.current = null;
    };
  }, [enabled, read]);

  const items = useMemo(
    () =>
      state.kind === "content"
        ? state.folders.map((smartFolder): LibraryItem => ({
            kind: "smartFolder",
            smartFolder,
          }))
        : noItems,
    [state],
  );
  return { state, items, retry: read };
}

/** Above the home's list; the cards are the library's, so selection and previews stay. */
export function FavoriteFolderBand({
  state,
  onRetry,
  cardProps,
}: {
  state: FavoriteFolders;
  onRetry: () => void;
  cardProps: (folder: SmartFolderSummary) => SmartFolderCardProps;
}) {
  if (state.kind === "none") return null;
  if (state.kind === "content" && state.folders.length === 0) return null;
  const all = (
    <Button variant="ghost" size="sm" asChild>
      <Link to="/smart-folders">
        {t.smartFolders.band.all}
        <ChevronRight aria-hidden="true" />
      </Link>
    </Button>
  );
  return (
    <div className="flex min-w-0 flex-col gap-3">
      {state.kind === "failed" ? (
        <Shelf title={t.smartFolders.band.title} actions={all}>
          <ErrorState
            title={t.smartFolders.listFailed(state.reason)}
            retryLabel={t.common.retry}
            onRetry={onRetry}
          />
        </Shelf>
      ) : (
        <Shelf
          title={t.smartFolders.band.title}
          count={t.smartFolders.band.count(state.folders.length)}
          actions={all}
          layout="row"
        >
          <SmartFolderCardsProvider>
            {state.folders.map((folder) => (
              <SmartFolderCard
                key={folder.id}
                {...cardProps(folder)}
                conditionChips={false}
              />
            ))}
          </SmartFolderCardsProvider>
        </Shelf>
      )}
      <Separator />
    </div>
  );
}
