import { CircleAlert, RotateCw } from "lucide-react";

import { t, type UiText } from "../i18n";
import { Alert, AlertAction, AlertTitle } from "../ui/shadcn/alert";
import { Button } from "../ui/shadcn/button";

/**
 * DialogError はタグ管理の窓の中の失敗の 1 行である（destructive の `Alert`）。
 * 窓は開いたままにし、やり直しが効くときは `onRetry` で「Retry」を添える
 * （web/registry/rules/components.md「Alert」）。
 */
export function DialogError({
  message,
  onRetry,
}: {
  message: UiText;
  onRetry?: () => void;
}) {
  return (
    <Alert variant="destructive">
      <CircleAlert aria-hidden="true" />
      <AlertTitle>{message}</AlertTitle>
      {onRetry !== undefined && (
        <AlertAction>
          <Button variant="outline" size="sm" onClick={onRetry}>
            <RotateCw aria-hidden="true" />
            {t.common.retry}
          </Button>
        </AlertAction>
      )}
    </Alert>
  );
}
