import { Shapes } from "lucide-react";

import type { TagKind } from "../api/tagKinds";
import type { Tag } from "../api/tags";
import { compareNatural } from "../api/tagOrder";
import { t } from "../i18n";
import { foldForMatch } from "../lib/foldForMatch";
import type { TagChoice } from "../ui/TagCommand";

interface Ranked<T> {
  item: T;
  name: string;
  prefix: boolean;
  /** The synonym that matched when the name did not. */
  synonym?: string;
}

/** Match by folded name or synonym; prefix matches first, then natural name order. */
function rankByName<T>(
  items: readonly T[],
  input: string,
  name: (item: T) => string,
  synonyms: (item: T) => readonly string[],
): Ranked<T>[] {
  const query = foldForMatch(input.trim());
  const ranked: Ranked<T>[] = [];
  for (const item of items) {
    const folded = foldForMatch(name(item));
    const nameMatch = folded.includes(query);
    const synonymHit = synonyms(item).find((synonym) =>
      foldForMatch(synonym).includes(query),
    );
    if (!nameMatch && synonymHit === undefined) continue;
    const prefix =
      folded.startsWith(query) ||
      (synonymHit !== undefined && foldForMatch(synonymHit).startsWith(query));
    ranked.push({
      item,
      name: name(item),
      prefix,
      synonym: nameMatch ? undefined : synonymHit,
    });
  }
  return ranked.sort(byRank);
}

function byRank(a: Ranked<unknown>, b: Ranked<unknown>): number {
  if (a.prefix !== b.prefix) return a.prefix ? -1 : 1;
  return compareNatural(a.name, b.name);
}

const noSynonyms = (): readonly string[] => [];

function tagChoice(tag: Tag, synonym?: string): TagChoice {
  return {
    id: String(tag.id),
    label: tag.name,
    hint: synonym === undefined ? undefined : t.library.selection.synonym(synonym),
    meta: t.library.selection.videoCount(tag.videoCount),
  };
}

function exactTag(allTags: readonly Tag[], trimmed: string): Tag | undefined {
  return allTags.find((tag) => tag.name === trimmed || tag.synonyms.includes(trimmed));
}

export function buildAddOptions(
  allTags: readonly Tag[],
  input: string,
): { options: TagChoice[]; exactOption: TagChoice | null } {
  const exact = exactTag(allTags, input.trim());
  const options = rankByName(
    allTags,
    input,
    (tag) => tag.name,
    (tag) => tag.synonyms,
  ).map(({ item, synonym }) => tagChoice(item, synonym));
  return { options, exactOption: exact === undefined ? null : tagChoice(exact) };
}

const kindIdPrefix = "kind:";

/** A choice made from a tag-or-kind list: a tag joins the tag filter, a kind replaces the kind filter. */
export type TagOrKindPick = { kind: "tag"; id: number } | { kind: "tagKind"; id: number };

export function pickOfChoice(choice: Pick<TagChoice, "id">): TagOrKindPick {
  return choice.id.startsWith(kindIdPrefix)
    ? { kind: "tagKind", id: Number(choice.id.slice(kindIdPrefix.length)) }
    : { kind: "tag", id: Number(choice.id) };
}

function kindChoice(kind: TagKind): TagChoice {
  return {
    id: `${kindIdPrefix}${String(kind.id)}`,
    label: kind.name,
    leading: <Shapes aria-hidden="true" className="size-4 text-muted-foreground" />,
    hint: t.library.tagFilter.kindHint,
    ariaLabel: t.library.tagFilter.kindOption(kind.name),
  };
}

/**
 * Tags and tag kinds in one list, ranked together; a kind row leads with the kind icon. An exact
 * tag spelling wins over a kind of the same name.
 */
export function buildTagAndKindOptions(
  allTags: readonly Tag[],
  kinds: readonly TagKind[],
  input: string,
): { options: TagChoice[]; exactOption: TagChoice | null } {
  const trimmed = input.trim();
  const ranked = [
    ...rankByName<Tag | TagKind>(
      allTags,
      input,
      (tag) => tag.name,
      (tag) => ("synonyms" in tag ? tag.synonyms : []),
    ),
    ...rankByName<Tag | TagKind>(kinds, input, (kind) => kind.name, noSynonyms),
  ].sort(byRank);
  const options = ranked.map(({ item, synonym }) =>
    "synonyms" in item ? tagChoice(item, synonym) : kindChoice(item),
  );
  const tag = exactTag(allTags, trimmed);
  const kind = kinds.find((each) => each.name === trimmed);
  const exactOption =
    tag !== undefined ? tagChoice(tag) : kind !== undefined ? kindChoice(kind) : null;
  return { options, exactOption };
}
