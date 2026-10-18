import { CircleAlert, LoaderCircle } from "lucide-react";

import type { UiText } from "../i18n";
import { Alert, AlertTitle } from "../ui/shadcn/alert";

export function SaveStatus({
  saving,
  savingText,
  error,
}: {
  saving: boolean;
  savingText: UiText;
  error: UiText | null;
}) {
  return (
    <>
      {saving && (
        <p
          role="status"
          className="flex items-center gap-2 text-xs text-muted-foreground"
        >
          <LoaderCircle
            aria-hidden="true"
            className="size-4 animate-spin motion-reduce:animate-none"
          />
          {savingText}
        </p>
      )}
      {error !== null && (
        <div>
          <Alert variant="destructive">
            <CircleAlert aria-hidden="true" />
            <AlertTitle>{error}</AlertTitle>
          </Alert>
        </div>
      )}
    </>
  );
}
