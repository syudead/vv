import { CircleAlert } from "lucide-react";
import type { ReactNode } from "react";

import { t, type UiText } from "../i18n";
import { FormDialog } from "../ui/patterns/form-dialog";
import { Alert, AlertTitle } from "../ui/shadcn/alert";

/**
 * WorkInfoPluginDialog は「Work-info plugins」節の承認の確認と設定のダイアログが共有する
 * `FormDialog` である。開いたまま描き、送っている間は閉じず、失敗を本文の最後の
 * `destructive` の `Alert` に出す（specs/049-work-info-plugins/ui-design.md）。
 */
export default function WorkInfoPluginDialog({
  title,
  description,
  submitLabel,
  pending,
  submitDisabled,
  failure,
  onClose,
  onSubmit,
  children,
}: {
  title: UiText;
  description: UiText;
  submitLabel: UiText;
  pending: boolean;
  submitDisabled: boolean;
  failure: UiText | null;
  onClose: () => void;
  /** 送れるとき（送っていず、押せないわけでもない）にだけ呼ぶ。 */
  onSubmit: () => void;
  children: ReactNode;
}) {
  return (
    <FormDialog
      open
      onOpenChange={(open) => {
        if (!open && !pending) onClose();
      }}
      title={title}
      description={description}
      submitLabel={submitLabel}
      cancelLabel={t.common.cancel}
      pending={pending}
      submitDisabled={submitDisabled}
      onSubmit={(event) => {
        event.preventDefault();
        if (!pending && !submitDisabled) onSubmit();
      }}
    >
      {children}
      {failure !== null && (
        <Alert variant="destructive">
          <CircleAlert aria-hidden="true" />
          <AlertTitle>{failure}</AlertTitle>
        </Alert>
      )}
    </FormDialog>
  );
}
