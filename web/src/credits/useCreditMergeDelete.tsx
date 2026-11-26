import { type ReactNode, useState } from "react";
import { useNavigate } from "react-router";

import {
  type Credit,
  type CreditKind,
  deleteCredit,
  isCreditGone,
  listCredits,
} from "../api/credits";
import { errorText, t, type UiText } from "../i18n";
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
 * The credit's video count as the list sums it across connected links by name; the single read
 * counts this node only. A nameless credit joins no link, and a failed read keeps this node's count.
 */
async function linkedVideoCount(kind: CreditKind, credit: Credit): Promise<number> {
  if (credit.name === "") return credit.videoCount;
  try {
    const list = await listCredits(kind, { q: credit.name });
    return (
      list.items.find((item) => item.id === credit.id)?.videoCount ?? credit.videoCount
    );
  } catch {
    return credit.videoCount;
  }
}
