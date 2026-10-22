import { Building2, Tag as TagIcon } from "lucide-react";
import { useEffect, useMemo, useState } from "react";

import {
  type Credit,
  type CreditKind,
  type CreditList,
  listCredits,
} from "../api/credits";
import { getTagKinds, type TagKind } from "../api/tagKinds";
import { getTags, type Tag } from "../api/tags";
import { creditFace } from "../credits/creditFace";
import { creditDisplayName } from "../credits/displayName";
import { t } from "../i18n";
import type {
  SearchSuggestions,
  SuggestionGroup,
  SuggestionOption,
} from "../videoList/SearchSuggestions";
import { type LibraryFilterPick } from "./filterPicks";
import { buildTagAndKindOptions, pickOfChoice } from "./tagChoices";

/** Matches shown per group. */
export const suggestionsPerGroup = 3;

export interface SuggestionSources {
  people: readonly Credit[];
  organizations: readonly Credit[];
  tags: readonly Tag[];
  kinds: readonly TagKind[];
}

const separator = ":";

function optionId(pick: LibraryFilterPick): string {
  return `${pick.kind}${separator}${String(pick.id)}`;
}

export function pickOfSuggestion(
  option: Pick<SuggestionOption, "id">,
): LibraryFilterPick {
  const at = option.id.indexOf(separator);
  const kind = option.id.slice(0, at) as LibraryFilterPick["kind"];
  return { kind, id: Number(option.id.slice(at + 1)) };
}

function creditOption(kind: CreditKind, credit: Credit): SuggestionOption {
  return {
    id: optionId({ kind, id: credit.id }),
    label: creditDisplayName(credit),
    leading:
      kind === "person" ? (
        creditFace(kind, credit, "option")
      ) : (
        <Building2 aria-hidden="true" className="text-muted-foreground" />
      ),
  };
}

/**
 * People, tags with tag kinds, and organizations matching the text, a few each, in that order;
 * tags and kinds match by folded name or synonym (foldForMatch), credits as the server matched
 * them. Empty groups are left out.
 */
export function suggestionGroups(
  text: string,
  sources: SuggestionSources,
): SuggestionGroup[] {
  const tagOptions = buildTagAndKindOptions(sources.tags, sources.kinds, text)
    .options.slice(0, suggestionsPerGroup)
    .map((choice): SuggestionOption => {
      const pick = pickOfChoice(choice);
      return pick.kind === "tag"
        ? {
            id: optionId(pick),
            label: choice.label,
            leading: <TagIcon aria-hidden="true" className="text-muted-foreground" />,
          }
        : {
            id: optionId(pick),
            label: t.list.suggestions.kind(choice.label),
            leading: choice.leading,
          };
    });
  const groups: SuggestionGroup[] = [
    {
      heading: t.list.suggestions.people,
      options: sources.people
        .slice(0, suggestionsPerGroup)
        .map((credit) => creditOption("person", credit)),
    },
    { heading: t.list.suggestions.tags, options: tagOptions },
    {
      heading: t.list.suggestions.organizations,
      options: sources.organizations
        .slice(0, suggestionsPerGroup)
        .map((credit) => creditOption("organization", credit)),
    },
  ];
  return groups.filter((group) => group.options.length > 0);
}

const noCredits: CreditList = { items: [], total: 0, totalAll: 0 };

function settled<T>(result: PromiseSettledResult<T>, fallback: T): T {
  return result.status === "fulfilled" ? result.value : fallback;
}

/** A failed source leaves its group out instead of hiding the others. */
async function loadSources(
  text: string,
  signal: AbortSignal,
): Promise<SuggestionSources> {
  const query = { q: text, limit: suggestionsPerGroup };
  const [people, organizations, tags, kinds] = await Promise.allSettled([
    listCredits("person", query, signal),
    listCredits("organization", query, signal),
    getTags(),
    getTagKinds(),
  ]);
  return {
    people: settled(people, noCredits).items,
    organizations: settled(organizations, noCredits).items,
    tags: settled(tags, []),
    kinds: settled(kinds, []),
  };
}

/**
 * Suggestions for the library's search field, or undefined when disabled (guests). A newer text
 * aborts the previous credit requests, and a response for an older text is never shown.
 */
export function useSearchSuggestions(
  enabled: boolean,
  onPick: (pick: LibraryFilterPick) => void,
): SearchSuggestions | undefined {
  const [term, setTerm] = useState("");
  const [found, setFound] = useState<{ term: string; groups: SuggestionGroup[] }>({
    term: "",
    groups: [],
  });

  useEffect(() => {
    if (!enabled || term === "") return;
    const controller = new AbortController();
    void loadSources(term, controller.signal).then((sources) => {
      if (!controller.signal.aborted) {
        setFound({ term, groups: suggestionGroups(term, sources) });
      }
    });
    return () => controller.abort();
  }, [enabled, term]);

  return useMemo(
    () =>
      enabled
        ? {
            term: found.term,
            groups: found.groups,
            onTermChange: setTerm,
            onPick: (option: SuggestionOption) => onPick(pickOfSuggestion(option)),
          }
        : undefined,
    [enabled, found, onPick],
  );
}
