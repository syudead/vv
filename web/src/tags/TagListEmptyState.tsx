import {
  CircleDashed,
  Plus,
  SearchX,
  Shapes,
  Tags as TagsIcon,
  VideoOff,
} from "lucide-react";

import { t, type UiText } from "../i18n";
import type { TagListEmpty } from "../recordList/listView";
import { TagsEmpty } from "../recordList/TagsEmpty";
import { Button } from "../ui/shadcn/button";
import type { TagKindFilter } from "./tagListUrl";

function noKindTitle(kind: TagKindFilter, kindName: string | undefined): UiText {
  if (kind === "none") return t.tags.noKindlessTags;
  return kindName === undefined ? t.tags.noFilterMatches : t.tags.noKindTags(kindName);
}

/** Describe the conditions of the received rows, not a pending query; Show all tags clears the search and every filter. */
export default function TagListEmptyState({
  empty,
  appliedSearch,
  appliedTentative,
  appliedKind,
  kindName,
  onCreate,
  onShowAll,
}: {
  empty: TagListEmpty;
  appliedSearch: string;
  appliedTentative: boolean;
  appliedKind: TagKindFilter;
  kindName: string | undefined;
  onCreate: () => void;
  onShowAll: () => void;
}) {
  const showAll = (
    <Button variant="outline" size="sm" onClick={onShowAll}>
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
          action={showAll}
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
          action={showAll}
        />
      );
    case "noTentative":
      return (
        <TagsEmpty
          icon={<CircleDashed aria-hidden="true" />}
          title={t.tags.noTentative.title}
          description={t.tags.noTentative.description}
          action={showAll}
        />
      );
    case "noTentativeMatch":
      return (
        <TagsEmpty
          icon={<SearchX aria-hidden="true" />}
          title={t.tags.noTentativeMatches(appliedSearch)}
          action={showAll}
        />
      );
    case "noKind":
      return (
        <TagsEmpty
          icon={<Shapes aria-hidden="true" />}
          title={noKindTitle(appliedKind, kindName)}
          action={showAll}
        />
      );
    case "noKindMatch":
      return (
        <TagsEmpty
          icon={<SearchX aria-hidden="true" />}
          title={t.tags.noFilterMatches}
          action={showAll}
        />
      );
    case "noMatch":
      return (
        <TagsEmpty
          icon={<SearchX aria-hidden="true" />}
          title={t.tags.noMatches(appliedSearch)}
          action={showAll}
        />
      );
  }
}
