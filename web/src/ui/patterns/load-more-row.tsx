import { CircleAlert, RotateCw } from "lucide-react";
import type { ReactNode } from "react";

import { Alert, AlertAction, AlertDescription, AlertTitle } from "@/ui/shadcn/alert";
import { Button } from "@/ui/shadcn/button";
import { Spinner } from "@/ui/shadcn/spinner";

// Retain existing content on pagination failure and retry the same page; see web/registry/rules/patterns.md, States.

export type LoadMoreRowProps =
  | {
      status: "loading";
      label: string;
    }
  | {
      status: "failed";
      title: ReactNode;
      description?: ReactNode;
      retryLabel: ReactNode;
      onRetry: () => void;
    };

export function LoadMoreRow(props: LoadMoreRowProps) {
  if (props.status === "loading") {
    return (
      <div
        data-slot="load-more-row"
        data-state="loading"
        className="flex justify-center py-3"
      >
        <Spinner aria-label={props.label} />
      </div>
    );
  }
  return (
    <Alert data-slot="load-more-row" data-state="failed" variant="destructive">
      <CircleAlert aria-hidden="true" />
      <AlertTitle>{props.title}</AlertTitle>
      {props.description && <AlertDescription>{props.description}</AlertDescription>}
      <AlertAction>
        <Button variant="outline" size="sm" onClick={props.onRetry}>
          <RotateCw aria-hidden="true" />
          {props.retryLabel}
        </Button>
      </AlertAction>
    </Alert>
  );
}
