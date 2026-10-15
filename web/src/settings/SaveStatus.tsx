import { CircleAlert, LoaderCircle } from "lucide-react";

import type { UiText } from "../i18n";
import { Alert, AlertTitle } from "../ui/shadcn/alert";

/**
 * SaveStatus は、押すとすぐに保存する設定のスイッチの下に出す保存の状態である。保存の最中は
 * 回る `LoaderCircle` と「Saving…」を読み上げに出し、失敗すると理由を destructive の Alert に出す。
 * 「Auto-import」「Network」「Duplicate judging」の区画が共有する。
 */
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
