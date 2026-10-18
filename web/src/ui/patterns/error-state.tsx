import { CircleAlert, RotateCw } from "lucide-react";
import type { ReactNode } from "react";

import { Alert, AlertDescription, AlertTitle } from "@/ui/shadcn/alert";
import { Button } from "@/ui/shadcn/button";

// See web/registry/rules/patterns.md, States.

export interface ErrorStateProps {
  title: ReactNode;
  description?: ReactNode;
  /** Supply together with onRetry. */
  retryLabel?: ReactNode;
  /** Omit retry for failures it cannot resolve. */
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
