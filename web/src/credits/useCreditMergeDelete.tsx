import { type ReactNode, useState } from "react";
import { useNavigate } from "react-router";

import { type Credit, type CreditKind, deleteCredit, isCreditGone } from "../api/credits";
import { errorText, t, type UiText } from "../i18n";
import { useToast } from "../ui/Toast";
import DeleteCreditDialog from "./DeleteCreditDialog";
import { creditDisplayName } from "./displayName";
import MergeCreditDialog from "./MergeCreditDialog";

/**
 * A profile page's merge and delete dialogs: a merge opens the target's page and a delete the
 * list, each with a toast; a missing record calls markGone.
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
  const [merging, setMerging] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<UiText | null>(null);

  function merged(target: Credit) {
    setMerging(false);
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

  const dialogs = (
    <>
      {merging && (
        <MergeCreditDialog
          kind={kind}
          source={credit}
          onClose={() => setMerging(false)}
          onMerged={merged}
          onStale={markGone}
        />
      )}
      {deleting && (
        <DeleteCreditDialog
          kind={kind}
          credit={credit}
          pending={pending}
          error={error}
          onClose={() => {
            if (!pending) setDeleting(false);
          }}
          onDelete={() => void performDelete()}
        />
      )}
    </>
  );

  return {
    openMerge: () => setMerging(true),
    openDelete: () => {
      setError(null);
      setDeleting(true);
    },
    dialogs,
  };
}
