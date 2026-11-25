import { useEffect, useState } from "react";

import { listTagPage, maxTagPageLimit, type Tag } from "../api/tags";
import { errorText, t, type UiText } from "../i18n";
import { foldForMatch } from "../lib/foldForMatch";
import { isRemoteId } from "../lib/remoteId";
import type { TagChoice } from "../ui/TagCommand";

const targetCandidateLimit = 8;

export interface TagTargetOptions {
  tags: Tag[];
  options: TagChoice[];
  exactOption: TagChoice | null;
}

/** Prioritize exact only when it still matches current input; otherwise preserve server order and show matched-synonym hints. */
function buildTargetOptions(
  items: readonly Tag[],
  exact: Tag | null,
  allowed: (tag: Tag) => boolean,
  input: string,
): TagTargetOptions {
  const trimmed = input.trim();
  const query = foldForMatch(trimmed);
  const spelled = (tag: Tag) =>
    trimmed !== "" && (tag.name === trimmed || tag.synonyms.includes(trimmed));
  const candidates = items.filter(allowed);

  const exactTag =
    exact !== null && allowed(exact) && spelled(exact) ? exact : candidates.find(spelled);
  const shown = (
    exactTag === undefined
      ? candidates
      : [exactTag, ...candidates.filter((tag) => tag.id !== exactTag.id)]
  ).slice(0, targetCandidateLimit);

  const options: TagChoice[] = shown.map((tag) => {
    const nameMatch = query === "" || foldForMatch(tag.name).includes(query);
    const synonymHit = nameMatch
      ? undefined
      : tag.synonyms.find((synonym) => foldForMatch(synonym).includes(query));
    return {
      id: String(tag.id),
      label: tag.name,
      hint:
        synonymHit !== undefined ? t.tags.mergeDialog.synonymHint(synonymHit) : undefined,
      meta: t.tags.mergeDialog.videoCount(tag.videoCount),
    };
  });

  const exactOption: TagChoice | null =
    exactTag === undefined
      ? null
      : {
          id: String(exactTag.id),
          label: exactTag.name,
          meta: t.tags.mergeDialog.videoCount(exactTag.videoCount),
        };

  return { tags: shown, options, exactOption };
}

/**
 * Search every server tag for a merge or move target, exact matches first. `excluded` tags never
 * show; `localOnly` also leaves out tags only the paired VVMDM holds, which a move cannot name.
 */
export function useTagTargetSearch(
  value: string,
  excluded: ReadonlySet<number>,
  localOnly = false,
): TagTargetOptions & { searching: boolean; searchError: UiText | null } {
  const [candidates, setCandidates] = useState<readonly Tag[]>([]);
  const [exactCandidate, setExactCandidate] = useState<Tag | null>(null);
  const [searching, setSearching] = useState(false);
  const [searchError, setSearchError] = useState<UiText | null>(null);
  // Overfetch so eight options remain after leaving out the excluded tags.
  const requestLimit = Math.min(targetCandidateLimit + excluded.size, maxTagPageLimit);

  useEffect(() => {
    const controller = new AbortController();
    setSearching(true);
    setSearchError(null);
    listTagPage({ q: value.trim(), limit: requestLimit }, controller.signal)
      .then((page) => {
        if (controller.signal.aborted) return;
        setCandidates(page.items);
        setExactCandidate(page.exact ?? null);
        setSearching(false);
      })
      .catch((failure: unknown) => {
        if (controller.signal.aborted) return;
        setSearchError(errorText(failure));
        setSearching(false);
      });
    return () => controller.abort();
  }, [value, requestLimit]);

  const allowed = (tag: Tag) =>
    !excluded.has(tag.id) && !(localOnly && isRemoteId(tag.id));
  return {
    ...buildTargetOptions(candidates, exactCandidate, allowed, value),
    searching,
    searchError,
  };
}
