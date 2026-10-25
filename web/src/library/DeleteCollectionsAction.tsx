import { Trash2 } from "lucide-react";
import { useId, useState } from "react";

import { type CollectionsDeleteResponse, deleteCollections } from "../api/collections";
import GroupConfirmDialog from "../groups/GroupConfirmDialog";
import { t, type UiText } from "../i18n";
import { useToast } from "../ui/Toast";
import { Separator } from "../ui/shadcn/separator";
import {
  collectionTitleIn,
  type RemotePartial,
  remotePartial,
} from "./RemotePartialDialog";
import {
  DisabledReason,
  disabledProps,
  SelectionAction,
  WithTooltip,
} from "./SelectionAction";
import type { SelectionTarget } from "./selection";

interface Targets {
  groupIds: readonly number[];
  smartFolderIds: readonly number[];
  /** The cards as the list showed them, for naming what the peer did not delete. */
  cards: readonly SelectionTarget[];
}

function deletedMessage(result: CollectionsDeleteResponse): UiText {
  const words = t.library.selection;
  return result.dissolvedGroups + result.deletedSmartFolders === 0
    ? words.collectionsGone
    : words.collectionsDeleted(result.dissolvedGroups, result.deletedSmartFolders);
}

/**
 * Dissolve the selected groups and delete the selected smart folders after confirmation; selected
 * videos are never deleted. Failures stay in the dialog, so the selection is kept.
 */
export default function DeleteCollectionsAction({
  groupIds,
  smartFolderIds,
  cards,
  disabledReason,
  onDeleted,
  onPartial,
}: Targets & {
  disabledReason: UiText | undefined;
  onDeleted: () => void;
  /** The peer kept some collections; the bar names them in place of the toast. */
  onPartial: (partial: RemotePartial) => void;
}) {
  const words = t.library.selection.deleteDialog;
  const toast = useToast();
  const reasonId = useId();
  // Freeze the targets at opening; the modal dialog keeps the selection from changing meanwhile.
  const [targets, setTargets] = useState<Targets | null>(null);

  return (
    <>
      <Separator orientation="vertical" />
      <WithTooltip label={disabledReason ?? t.library.selection.deleteCollections}>
        <SelectionAction
          variant="ghost-destructive"
          icon={<Trash2 aria-hidden="true" />}
          label={t.library.selection.deleteCollections}
          {...disabledProps(disabledReason, reasonId)}
          onClick={() => setTargets({ groupIds, smartFolderIds, cards })}
        />
      </WithTooltip>
      <DisabledReason id={reasonId} reason={disabledReason} />
      {targets !== null && (
        <GroupConfirmDialog
          request={() => deleteCollections(targets.groupIds, targets.smartFolderIds)}
          onDone={(result) => {
            setTargets(null);
            const partial = remotePartial(
              result.remote,
              collectionTitleIn(targets.cards),
              "deleted",
            );
            if (partial === null) toast(deletedMessage(result));
            onDeleted();
            if (partial !== null) onPartial(partial);
          }}
          onClose={() => setTargets(null)}
          // The endpoint ignores missing targets, so it never answers 404.
          onGone={() => setTargets(null)}
          title={words.title(targets.groupIds.length, targets.smartFolderIds.length)}
          description={
            <>
              {targets.groupIds.length > 0 && words.groups(targets.groupIds.length)}
              {targets.groupIds.length > 0 && targets.smartFolderIds.length > 0 && " "}
              {targets.smartFolderIds.length > 0 &&
                words.smartFolders(targets.smartFolderIds.length)}
            </>
          }
          submit={words.submit}
          submitting={words.submitting}
          failed={words.failed}
        />
      )}
    </>
  );
}
