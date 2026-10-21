import { useEffect, useState } from "react";

import { isAborted } from "../api/client";
import { type Credit, type CreditKind, listCredits } from "../api/credits";
import { errorText, t, type UiText } from "../i18n";
import { foldForMatch } from "../lib/foldForMatch";
import type { CreditChoice } from "../ui/CreditCommand";
import { creditFace } from "./creditFace";
import { creditDisplayName } from "./displayName";

const candidateLimit = 8;

/**
 * Credits matching the typed name, the source left out, as choices for picking where something goes. Fetches one
 * extra candidate to keep eight after leaving out the source, and aborts obsolete searches.
 */
export function useCreditTargetSearch(kind: CreditKind, sourceId: number, value: string) {
  const [candidates, setCandidates] = useState<readonly Credit[]>([]);
  const [searching, setSearching] = useState(false);
  const [searchError, setSearchError] = useState<UiText | null>(null);

  useEffect(() => {
    const controller = new AbortController();
    setSearching(true);
    setSearchError(null);
    listCredits(
      kind,
      { q: value.trim(), limit: candidateLimit + 1 },
      controller.signal,
    ).then(
      (page) => {
        if (controller.signal.aborted) return;
        setCandidates(page.items);
        setSearching(false);
      },
      (failure: unknown) => {
        if (controller.signal.aborted || isAborted(failure)) return;
        setSearchError(errorText(failure));
        setSearching(false);
      },
    );
    return () => controller.abort();
  }, [kind, value]);

  const shown = candidates
    .filter((credit) => credit.id !== sourceId)
    .slice(0, candidateLimit);
  const folded = foldForMatch(value.trim());
  const options: CreditChoice[] = shown.map((credit) => {
    const alias =
      folded === "" || foldForMatch(credit.name).includes(folded)
        ? undefined
        : credit.aliases.find((item) => foldForMatch(item.name).includes(folded))?.name;
    return {
      id: String(credit.id),
      label: creditDisplayName(credit),
      leading: creditFace(kind, credit, "option"),
      hint: alias === undefined ? undefined : t.credits.mergeDialog.aliasHint(alias),
      meta: t.credits.mergeDialog.videoCount(credit.videoCount),
    };
  });
  const trimmed = value.trim();
  const exact = shown.find((credit) => trimmed !== "" && credit.name === trimmed);
  const exactOption =
    exact === undefined
      ? null
      : (options.find((option) => option.id === String(exact.id)) ?? null);
  return { shown, options, exactOption, searching, searchError };
}
