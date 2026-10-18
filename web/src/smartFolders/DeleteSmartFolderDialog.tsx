import { CircleAlert, RotateCw } from "lucide-react";
import { useEffect, useState } from "react";

import { deleteSmartFolder, listSmartFolders } from "../api/smartFolders";
import GroupConfirmDialog from "../groups/GroupConfirmDialog";
import { errorText, t, type UiText } from "../i18n";
import { Alert, AlertAction, AlertTitle } from "../ui/shadcn/alert";
import { Button } from "../ui/shadcn/button";
import { descendantIds } from "./smartRule";

function useDescendantCount(id: number) {
  const [count, setCount] = useState<number | undefined>();
  const [failure, setFailure] = useState<UiText | null>(null);
  const [round, setRound] = useState(0);
  useEffect(() => {
    const controller = new AbortController();
    listSmartFolders(controller.signal).then(
      (folders) => setCount(descendantIds(folders, id).size),
      (error: unknown) => {
        if (!controller.signal.aborted) setFailure(errorText(error));
      },
    );
    return () => controller.abort();
  }, [id, round]);
  const retry = () => {
    setFailure(null);
    setRound((value) => value + 1);
  };
  return { count, failure, retry };
}

/**
 * Mount only while open and gate deletion on a successful descendant count. A missing folder calls
 * onDeleted(false) to suppress the success toast; other failures remain in the dialog.
 */
export default function DeleteSmartFolderDialog({
  folder,
  onClose,
  onDeleted,
}: {
  folder: { id: number; name: string };
  onClose: () => void;
  /** False means already missing, so do not announce deletion. */
  onDeleted: (announce: boolean) => void;
}) {
  const words = t.smartFolders.deleteDialog;
  const descendants = useDescendantCount(folder.id);
  const count = descendants.count;

  return (
    <GroupConfirmDialog
      request={() => deleteSmartFolder(folder.id)}
      onDone={() => onDeleted(true)}
      onClose={onClose}
      onGone={() => onDeleted(false)}
      title={words.title(folder.name)}
      description={
        count === undefined || count === 0 ? (
          words.description
        ) : (
          <>
            {words.description} {words.descendants(count)}
          </>
        )
      }
      actionDisabled={count === undefined}
      notice={
        descendants.failure !== null && (
          <Alert variant="destructive">
            <CircleAlert aria-hidden="true" />
            <AlertTitle>{t.smartFolders.listFailed(descendants.failure)}</AlertTitle>
            <AlertAction>
              <Button
                type="button"
                variant="outline"
                size="sm"
                onClick={descendants.retry}
              >
                <RotateCw aria-hidden="true" />
                {t.common.retry}
              </Button>
            </AlertAction>
          </Alert>
        )
      }
      submit={words.submit}
      submitting={words.submitting}
      failed={words.failed}
    />
  );
}
