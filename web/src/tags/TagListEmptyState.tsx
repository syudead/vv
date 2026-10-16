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

/**
 * noKindTitle は種類の絞り込みだけで行が無いときの見出しである
 * （specs/051-tag-kinds/ui-design.md「Words」）。種類の名前をまだ読んでいなければ、
 * 組み合わせの見出しを使う。
 */
function noKindTitle(kind: TagKindFilter, kindName: string | undefined): UiText {
  if (kind === "none") return t.tags.noKindlessTags;
  return kindName === undefined ? t.tags.noFilterMatches : t.tags.noKindTags(kindName);
}

/**
 * TagListEmptyState は一覧の代わりに出す空の状態である（ui-design.md「States」）。
 * 文言は届いた行の条件（`appliedSearch`・`appliedTentative`・`appliedKind`）で決める。
 */
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
  /** 届いた行の種類の絞り込み。 */
  appliedKind: TagKindFilter;
  /** `appliedKind` の種類の名前。読み込み前（と `none`）は undefined。 */
  kindName: string | undefined;
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

/**
 * TagsEmpty は見出しつきの空の状態である。題は見出し（h2）にし、読み上げソフトで
 * 見出しから状態へ飛べるようにする（状態の部品の題は見出しの要素を持たないため）。
 */
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
