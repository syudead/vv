import { CircleAlert, RotateCw } from "lucide-react";
import type { ReactNode } from "react";

import { Alert, AlertDescription, AlertTitle } from "@/ui/shadcn/alert";
import { Button } from "@/ui/shadcn/button";

// エラー（状態）。骨格の本体の位置に、destructive の Alert で何が失敗したかと、やり直しが
// 効くときの Retry を出す。規則は web/registry/rules/patterns.md の States。

export interface ErrorStateProps {
  /** 何が失敗したか（「Couldn't load videos」）。 */
  title: ReactNode;
  /** 理由と、見る人にできること。 */
  description?: ReactNode;
  /** Retry の文言。onRetry と一緒に渡す。 */
  retryLabel?: ReactNode;
  /** 同じ要求をもう一度送る。やり直しが効かない失敗では渡さない。 */
  onRetry?: () => void;
}

export function ErrorState({ title, description, retryLabel, onRetry }: ErrorStateProps) {
  return (
    <Alert data-state="error" variant="destructive">
      <CircleAlert aria-hidden="true" />
      <AlertTitle>{title}</AlertTitle>
      {(description || onRetry) && (
        <AlertDescription className="flex flex-col items-start gap-2">
          {description && <p>{description}</p>}
          {onRetry && (
            <Button variant="outline" size="sm" onClick={onRetry}>
              <RotateCw aria-hidden="true" />
              {retryLabel}
            </Button>
          )}
        </AlertDescription>
      )}
    </Alert>
  );
}
