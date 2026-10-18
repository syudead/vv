import {
  CircleDashed,
  Plus,
  SearchX,
  Shapes,
  Tags as TagsIcon,
  VideoOff,
} from "lucide-react";
import type { ReactNode } from "react";

import { t, type UiText } from "../i18n";
import { EmptyState } from "../ui/patterns/empty-state";
import { Button } from "../ui/shadcn/button";
import type { TagKindFilter } from "./tagListUrl";
import type { TagListEmpty } from "./tagListView";

function noKindTitle(kind: TagKindFilter, kindName: string | undefined): UiText {
  if (kind === "none") return t.tags.noKindlessTags;
  return kindName === undefined ? t.tags.noFilterMatches : t.tags.noKindTags(kindName);
}

/** Describe the conditions of the received rows, not a pending query. */
export default function TagListEmptyState({
  empty,
  appliedSearch,
  appliedTentative,
  appliedKind,
  kindName,
  onCreate,
  onShowAll,
  onShowAllAndSearch,
  onClearSearch,
}: {
  empty: TagListEmpty;
  appliedSearch: string;
  appliedTentative: boolean;
  appliedKind: TagKindFilter;
  kindName: string | undefined;
  onCreate: () => void;
  onShowAll: () => void;
  onShowAllAndSearch: () => void;
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
    case "noKind":
      return (
        <TagsEmpty
          icon={<Shapes aria-hidden="true" />}
          title={noKindTitle(appliedKind, kindName)}
          action={showAll(onShowAll)}
        />
      );
    case "noKindMatch":
      return (
        <TagsEmpty
          icon={<SearchX aria-hidden="true" />}
          title={t.tags.noFilterMatches}
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

/** Use h2 because the shared state title has no heading semantics. */
export function TagsEmpty({
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
