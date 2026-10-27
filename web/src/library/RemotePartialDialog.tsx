import type { components } from "../api/gen/openapi";
import { errorReasonText, t } from "../i18n";
import { Button } from "../ui/shadcn/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "../ui/shadcn/dialog";
import type { SelectionTarget } from "./selection";

export type RemoteSelectionResult = components["schemas"]["RemoteSelectionResult"];

/** What the peer did not change, named as the list showed it when the request was sent. */
export interface RemotePartial {
  kind: "changed" | "deleted";
  failedIds: readonly number[];
  titles: readonly string[];
  reason: string;
}

/** A failed video's title from the sent cards: its own card, else the group card that holds it. */
export function videoTitleIn(targets: readonly SelectionTarget[]) {
  return (id: number): string | undefined =>
    (
      targets.find((target) => target.kind === "video" && target.id === id) ??
      targets.find((target) => target.kind === "group" && target.videoIds.includes(id))
    )?.label;
}

/** A failed collection's name from the sent group and smart-folder cards. */
export function collectionTitleIn(targets: readonly SelectionTarget[]) {
  return (id: number): string | undefined =>
    targets.find((target) => target.kind !== "video" && target.id === id)?.label;
}

/**
 * The peer's failures of one operation, or null when the peer changed everything it was sent. A
 * failed id that no sent card names, such as a smart folder's member, keeps its number.
 */
export function remotePartial(
  remote: RemoteSelectionResult | undefined,
  titleOf: (id: number) => string | undefined,
  kind: RemotePartial["kind"],
): RemotePartial | null {
  if (remote === undefined || remote.failedVideoIds.length === 0) return null;
  const titles: string[] = [];
  const seen = new Set<string>();
  for (const id of remote.failedVideoIds) {
    const title = titleOf(id) ?? t.library.selection.unknownTarget(id);
    if (seen.has(title)) continue;
    seen.add(title);
    titles.push(title);
  }
  return {
    kind,
    failedIds: remote.failedVideoIds,
    titles,
    reason:
      remote.reason === undefined
        ? t.errors.reason.peer_unreachable
        : errorReasonText(remote.reason),
  };
}

/** Names what the peer did not change; it does not depend on the list, which may no longer show them. */
export default function RemotePartialDialog({
  partial,
  peerName,
  onClose,
}: {
  partial: RemotePartial;
  peerName: string;
  onClose: () => void;
}) {
  const words = t.library.selection.remotePartial;
  const count = partial.failedIds.length;
  const changed = partial.kind === "changed";
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
    >
      <DialogContent showCloseButton={false}>
        <DialogHeader>
          <DialogTitle>
            {changed ? words.changedTitle(peerName) : words.deletedTitle(peerName)}
          </DialogTitle>
          <DialogDescription>
            {changed
              ? words.changed(count, peerName, partial.reason)
              : words.deleted(count, peerName, partial.reason)}
          </DialogDescription>
        </DialogHeader>
        <ul
          aria-label={changed ? words.changedList : words.deletedList}
          className="flex max-h-dialog-list flex-col overflow-y-auto text-sm"
        >
          {partial.titles.map((title) => (
            <li key={title} className="truncate" title={title}>
              {title}
            </li>
          ))}
        </ul>
        <DialogFooter>
          <Button size="sm" onClick={onClose}>
            {words.close}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
