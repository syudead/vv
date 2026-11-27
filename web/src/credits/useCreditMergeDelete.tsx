import { type ReactNode, useState } from "react";
import { useNavigate } from "react-router";

import {
  type Credit,
  type CreditKind,
  deleteCredit,
  getCredit,
  isCreditGone,
  listCredits,
} from "../api/credits";
import { errorText, t, type UiText } from "../i18n";
import { isRemoteId } from "../lib/remoteId";
import { useToast } from "../ui/Toast";
import DeleteCreditDialog from "./DeleteCreditDialog";
import { creditDisplayName } from "./displayName";
import MergeCreditDialog from "./MergeCreditDialog";

/**
 * A profile page's merge and delete dialogs: a merge opens the target's page and a delete the
 * list, each with a toast; a missing record calls markGone. The dialogs count the videos the list
 * sums across connected links, since both actions also change a link's credit of the same name.
 */
export function useCreditMergeDelete({
  kind,
  credit,
  pageOf,
  listPath,
  markGone,
}: {
  kind: CreditKind;
  credit: Credit;
  pageOf: (id: number) => string;
  listPath: string;
  markGone: () => void;
}): { openMerge: () => void; openDelete: () => void; dialogs: ReactNode } {
  const toast = useToast();
  const navigate = useNavigate();
  // The open dialog and the video count it confirms.
  const [open, setOpen] = useState<{
    dialog: "merge" | "delete";
    videoCount: number;
  } | null>(null);
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<UiText | null>(null);

  function merged(target: Credit) {
    setOpen(null);
    toast(t.credits.personPage.mergedInto(creditDisplayName(target)));
    void navigate(pageOf(target.id));
  }

  async function performDelete() {
    setError(null);
    setPending(true);
    try {
      await deleteCredit(kind, credit.id);
      toast(t.credits.deleted(creditDisplayName(credit)));
      void navigate(listPath);
    } catch (failure) {
      if (isCreditGone(failure)) {
        markGone();
        return;
      }
      setError(errorText(failure));
      setPending(false);
    }
  }

  async function openDialog(dialog: "merge" | "delete") {
    const videoCount = await linkedVideoCount(kind, credit);
    setError(null);
    setOpen({ dialog, videoCount });
  }

  const counted = open === null ? credit : { ...credit, videoCount: open.videoCount };
  const dialogs = (
    <>
      {open?.dialog === "merge" && (
        <MergeCreditDialog
          kind={kind}
          source={counted}
          onClose={() => setOpen(null)}
          onMerged={merged}
          onStale={markGone}
        />
      )}
      {open?.dialog === "delete" && (
        <DeleteCreditDialog
          kind={kind}
          credit={counted}
          pending={pending}
          error={error}
          onClose={() => {
            if (!pending) setOpen(null);
          }}
          onDelete={() => void performDelete()}
        />
      )}
    </>
  );

  return {
    openMerge: () => void openDialog("merge"),
    openDelete: () => void openDialog("delete"),
    dialogs,
  };
}

/**
 * The credit's video count as the list sums it across links by name, found by id or, for a later
 * link's id, by name. A remote id's change keeps this node's entry of the name, whose count is taken
 * out; a nameless credit or a failed read keeps the single read's count of this node only.
 */
async function linkedVideoCount(kind: CreditKind, credit: Credit): Promise<number> {
  if (credit.name === "") return credit.videoCount;
  try {
    const list = await listCredits(kind, { q: credit.name });
    const item =
      list.items.find((entry) => entry.id === credit.id) ??
      list.items.find((entry) => entry.name === credit.name);
    if (item === undefined) return credit.videoCount;
    if (!isRemoteId(credit.id) || isRemoteId(item.id)) return item.videoCount;
    return item.videoCount - (await getCredit(kind, item.id)).videoCount;
  } catch {
    return credit.videoCount;
  }
}
