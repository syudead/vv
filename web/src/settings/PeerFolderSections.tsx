import { Folder } from "lucide-react";
import { useCallback, useEffect, useState } from "react";

import {
  getPeer,
  listPeerLinkMediaFolders,
  type PeerLinkState,
  type PeerMediaFolder,
  setPeerLinkMediaFolderEnabled,
} from "../api/peer";
import { subscribeServerEvents } from "../api/serverEvents";
import { t, type UiText } from "../i18n";
import { cn } from "../lib/cn";
import { EmptyState } from "../ui/patterns/empty-state";
import { ErrorState } from "../ui/patterns/error-state";
import { PageSection } from "../ui/patterns/page-section";
import { Skeleton } from "../ui/shadcn/skeleton";
import { FolderSwitch } from "./FolderSwitch";
import { PeerLinkBadge } from "./PeerLinkBadge";
import { useSettingsSource } from "./useSettingsSource";
import { useSwitchSave } from "./useSwitchSave";

type FoldersView =
  | { kind: "loading" }
  | { kind: "failed"; reason: UiText }
  | { kind: "loaded"; folders: PeerMediaFolder[] };

/**
 * The links in slot order, from GET /api/peer and then each peer event. A failed read shows no link
 * section: the Pairing page carries that failure, and the local section stays usable.
 */
function usePeerLinks(): PeerLinkState[] {
  const [links, setLinks] = useState<PeerLinkState[]>([]);
  useEffect(() => {
    let changed = false;
    const apply = (next: PeerLinkState[]) =>
      setLinks([...next].sort((a, b) => a.slot - b.slot));
    // Subscribe before reading: an event that arrives while the read is in flight is newer than its answer.
    const unsubscribe = subscribeServerEvents({
      peer: (state) => {
        changed = true;
        apply(state.links);
      },
    });
    const controller = new AbortController();
    getPeer(controller.signal).then(
      (state) => {
        if (!changed) apply(state.links);
      },
      () => undefined,
    );
    return () => {
      unsubscribe();
      controller.abort();
    };
  }, []);
  return links;
}

/** No add, change, rescan or remove: a link's folders are its own, and the switch changes only this node. */
function PeerFolderRow({
  nodeId,
  name,
  folder,
  onSwitched,
  onStart,
  onSettled,
}: {
  nodeId: string;
  name: string;
  folder: PeerMediaFolder;
  onSwitched: (folder: PeerMediaFolder) => void;
  onStart: () => void;
  onSettled: () => void;
}) {
  const { saving, saveError, toggle } = useSwitchSave({
    settings: folder,
    current: (saved) => saved.enabled,
    save: (next) => setPeerLinkMediaFolderEnabled(nodeId, folder.id, next),
    saved: onSwitched,
    saveFailed: t.settings.mediaFolders.switchFailed,
    onStart,
    onSettled,
  });
  const enabled = saving ?? folder.enabled;
  return (
    <div className="flex min-w-0 flex-col gap-2 py-3 first:pt-0 last:pb-0">
      <div className="flex min-w-0 flex-wrap items-center gap-x-3 gap-y-2">
        <Folder
          className={cn(
            "size-4 shrink-0 text-muted-foreground",
            !enabled && "opacity-50",
          )}
        />
        <code
          className={cn(
            "block min-w-0 flex-1 text-sm break-all",
            !enabled && "text-muted-foreground",
          )}
          title={folder.path}
        >
          {folder.path}
        </code>
        <FolderSwitch
          enabled={enabled}
          saving={saving !== null}
          label={t.settings.mediaFolders.link.switchLabel(folder.path, name)}
          onToggle={() => void toggle()}
        />
      </div>
      {saveError !== null && (
        <p role="alert" className="text-destructive">
          {saveError}
        </p>
      )}
    </div>
  );
}

/**
 * Rereads in place on mediaFolders and peer, keeping the rows shown when a reread fails. Only the
 * latest read applies, and reads pause while a switch saves so an older answer cannot undo it.
 */
function usePeerLinkFolders(nodeId: string) {
  const read = useCallback(
    async (signal: AbortSignal) =>
      (await listPeerLinkMediaFolders(nodeId, signal)).folders,
    [nodeId],
  );
  const source = useSettingsSource(read);
  const { load } = source;

  useEffect(
    () =>
      subscribeServerEvents({
        mediaFolders: () => void load(),
        peer: () => void load(),
      }),
    [load],
  );

  const view: FoldersView =
    source.settings !== null
      ? { kind: "loaded", folders: source.settings }
      : source.loadError !== null
        ? { kind: "failed", reason: source.loadError }
        : { kind: "loading" };
  const replace = (switched: PeerMediaFolder) =>
    source.update((folders) =>
      folders.map((item) => (item.id === switched.id ? switched : item)),
    );

  return { view, source, replace };
}

/**
 * Drawn from this node's snapshot, so a link that is not connected keeps its rows; only the badge follows the link.
 * The name wraps as on the Pairing page, so a long name reads the same on both.
 */
function PeerFolderSection({ link }: { link: PeerLinkState }) {
  const text = t.settings.mediaFolders.link;
  const name = link.name ?? "";
  const nodeId = link.nodeId ?? "";
  const { view, source, replace } = usePeerLinkFolders(nodeId);
  return (
    <PageSection
      title={name}
      description={text.description(name)}
      actions={<PeerLinkBadge state={link.state} />}
    >
      <div aria-label={text.list(name)} className="flex flex-col divide-y divide-border">
        {view.kind === "loading" && (
          <div
            role="status"
            aria-label={text.loading(name)}
            className="flex flex-col gap-3"
          >
            <Skeleton className="h-10" />
            <Skeleton className="h-10" />
          </div>
        )}
        {view.kind === "failed" && (
          <ErrorState
            title={text.loadFailed(name, view.reason)}
            retryLabel={t.common.retry}
            onRetry={() => void source.load()}
          />
        )}
        {view.kind === "loaded" && view.folders.length === 0 && (
          <EmptyState
            icon={<Folder />}
            title={text.empty(name)}
            description={text.emptyHint}
          />
        )}
        {view.kind === "loaded" &&
          view.folders.map((folder) => (
            <PeerFolderRow
              key={folder.id}
              nodeId={nodeId}
              name={name}
              folder={folder}
              onSwitched={replace}
              onStart={source.beginSave}
              onSettled={source.endSave}
            />
          ))}
      </div>
    </PageSection>
  );
}

/** One section per link, in slot order, under the local Media folders section. */
export function PeerFolderSections() {
  const links = usePeerLinks();
  return links
    .filter((link) => link.nodeId !== undefined)
    .map((link) => <PeerFolderSection key={link.nodeId} link={link} />);
}
