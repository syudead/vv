import { CircleAlert, RotateCw } from "lucide-react";
import type { ReactNode } from "react";

import { Alert, AlertAction, AlertDescription, AlertTitle } from "@/ui/shadcn/alert";
import { Button } from "@/ui/shadcn/button";
import { Spinner } from "@/ui/shadcn/spinner";

// 追加読み込みの行（状態）。中身の後ろ、本体の末尾に置く。読み込み中は Spinner の行、
// 続きの読み込みに失敗したら中身を残したまま Retry つきの Alert の行を出し、Retry は
// 同じページを再要求する。規則は web/registry/rules/patterns.md の States。

export type LoadMoreRowProps =
  | {
      status: "loading";
      /** 読み込んでいるものの名前（「Loading more videos」）。 */
      label: string;
    }
  | {
      status: "failed";
      /** 何が失敗したか（「Couldn't load more videos」）。 */
      title: ReactNode;
      description?: ReactNode;
      retryLabel: ReactNode;
      /** 失敗した同じページを再要求する。 */
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
