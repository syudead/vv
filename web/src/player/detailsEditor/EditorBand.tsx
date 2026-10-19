import { AlertCircle, LoaderCircle } from "lucide-react";
import type { ReactNode, Ref } from "react";

import { t } from "../../i18n";
import { Button } from "../../ui/shadcn/button";
import type { SaveFailure } from "./useDetailsSave";

// The band actions and failure line every full-page editor shares.

export function EditorBand({
  saving,
  saveDisabled,
  onCancel,
  onSave,
  cancelRef,
}: {
  saving: boolean;
  saveDisabled: boolean;
  onCancel: () => void;
  onSave: () => void;
  cancelRef?: Ref<HTMLButtonElement>;
}) {
  const e = t.player.detailsEditor;
  return (
    <>
      <Button
        ref={cancelRef}
        variant="ghost"
        size="sm"
        aria-disabled={saving || undefined}
        onClick={() => {
          if (!saving) onCancel();
        }}
      >
        {e.cancel}
      </Button>
      <Button
        size="sm"
        aria-disabled={saving || saveDisabled || undefined}
        onClick={() => {
          if (!saving && !saveDisabled) onSave();
        }}
      >
        {saving && (
          <LoaderCircle
            className="animate-spin motion-reduce:animate-none"
            aria-hidden="true"
          />
        )}
        {saving ? e.saving : e.save}
      </Button>
    </>
  );
}

export function FailureLine({ children }: { children: ReactNode }) {
  return (
    <p role="alert" className="flex items-center gap-2 text-sm text-destructive">
      <AlertCircle className="size-4 shrink-0" aria-hidden="true" />
      <span className="min-w-0 wrap-anywhere">{children}</span>
    </p>
  );
}

export function failureText(failure: SaveFailure): ReactNode {
  if (failure.field === undefined) return failure.text;
  return (
    <>
      {failure.text} <code className="font-mono text-xs">{failure.field}</code>
    </>
  );
}
