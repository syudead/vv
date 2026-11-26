import type { CreditKind } from "../api/credits";
import {
  TagListChanged,
  TagLoadingMore,
  TagLoadMoreFailed,
} from "../recordList/TagListNotices";
import type { MoreState } from "../recordList/usePagedRows";
import { kindTexts } from "./creditRows";

export function CreditListTail({
  kind,
  tail,
  onRetryMore,
  onReload,
}: {
  kind: CreditKind;
  tail: MoreState;
  onRetryMore: () => void;
  onReload: () => void;
}) {
  const texts = kindTexts(kind);
  switch (tail.kind) {
    case "loading":
      return <TagLoadingMore label={texts.loadingMore} />;
    case "failed":
      return <TagLoadMoreFailed reason={tail.error} onRetry={onRetryMore} />;
    case "inconsistent":
      return <TagListChanged onReload={onReload} message={texts.listChanged} />;
    case "idle":
      return null;
  }
}
