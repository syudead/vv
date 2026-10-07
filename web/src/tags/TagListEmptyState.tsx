import { CircleDashed, Plus, SearchX, Tags as TagsIcon, VideoOff } from "lucide-react";
import type { ReactNode } from "react";

import { t, type UiText } from "../i18n";
import { EmptyState } from "../ui/patterns/empty-state";
import { Button } from "../ui/shadcn/button";
import type { TagListEmpty } from "./tagListView";

/**
 * TagListEmptyState は一覧の代わりに出す空の状態である（ui-design.md「States」）。
 * 文言は届いた行の条件（`appliedSearch`・`appliedTentative`）で決める。
 */
export default function TagListEmptyState({
  empty,
  appliedSearch,
  appliedTentative,
  onCreate,
  onShowAll,
  onShowAllAndSearch,
  onClearSearch,
}: {
  empty: TagListEmpty;
  appliedSearch: string;
  appliedTentative: boolean;
  /** 「新しいタグ」。 */
  onCreate: () => void;
  /** 絞り込みを外す「Show all tags」。 */
  onShowAll: () => void;
  /** 絞り込みと検索を外す「Show all tags」。 */
  onShowAllAndSearch: () => void;
  /** 検索を外す「Show all tags」。 */
  onClearSearch: () => void;
}) {
  const showAll = (onClick: () => void) => (
    <Button variant="outline" size="sm" onClick={onClick}>
      {t.tags.clearSearch}
    </Button>
  );
  switch (empty) {
    case "tags":
      return (
        <TagsEmpty
          icon={<TagsIcon aria-hidden="true" />}
          title={t.tags.empty.title}
          description={t.tags.empty.description}
          action={
            <Button size="sm" onClick={onCreate}>
              <Plus aria-hidden="true" />
              {t.tags.newTag}
            </Button>
          }
        />
      );
    case "noUnused":
      return (
        <TagsEmpty
          icon={<VideoOff aria-hidden="true" />}
          title={appliedTentative ? t.tags.noUnusedTentative : t.tags.noUnused.title}
          description={appliedTentative ? undefined : t.tags.noUnused.description}
          action={showAll(onShowAll)}
        />
      );
    case "noUnusedMatch":
      return (
        <TagsEmpty
          icon={<SearchX aria-hidden="true" />}
          title={
            appliedTentative
              ? t.tags.noUnusedTentativeMatches(appliedSearch)
              : t.tags.noUnusedMatches(appliedSearch)
          }
          action={showAll(onShowAllAndSearch)}
        />
      );
    case "noTentative":
      return (
        <TagsEmpty
          icon={<CircleDashed aria-hidden="true" />}
          title={t.tags.noTentative.title}
          description={t.tags.noTentative.description}
          action={showAll(onShowAll)}
        />
      );
    case "noTentativeMatch":
      return (
        <TagsEmpty
          icon={<SearchX aria-hidden="true" />}
          title={t.tags.noTentativeMatches(appliedSearch)}
          action={showAll(onShowAllAndSearch)}
        />
      );
    case "noMatch":
      return (
        <TagsEmpty
          icon={<SearchX aria-hidden="true" />}
          title={t.tags.noMatches(appliedSearch)}
          action={showAll(onClearSearch)}
        />
      );
  }
}

/**
 * TagsEmpty は見出しつきの空の状態である。題は見出し（h2）にし、読み上げソフトで
 * 見出しから状態へ飛べるようにする（状態の部品の題は見出しの要素を持たないため）。
 */
function TagsEmpty({
  icon,
  title,
  description,
  action,
}: {
  icon: ReactNode;
  title: UiText;
  description?: UiText;
  action: ReactNode;
}) {
  return (
    <EmptyState
      icon={icon}
      title={<h2>{title}</h2>}
      description={description}
      action={action}
    />
  );
}
