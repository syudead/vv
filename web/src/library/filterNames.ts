import { useEffect, useEffectEvent, useRef, useState } from "react";

import { RequestFailed } from "../api/client";
import { type CreditKind, getCredit } from "../api/credits";
import {
  currentTags,
  refreshTags,
  revalidateTags,
  subscribeTags,
  type Tag,
} from "../api/tags";

export function useTagNames(tagIds: readonly number[]): Map<number, Tag> {
  const [allTags, setAllTags] = useState<Tag[] | undefined>(currentTags());
  // Wait for mount revalidation before treating cached missing IDs as deleted.
  const [revalidated, setRevalidated] = useState(false);
  useEffect(() => {
    let alive = true;
    revalidateTags()
      .then((loaded) => {
        if (!alive) return;
        setAllTags(loaded);
        setRevalidated(true);
      })
      .catch(() => undefined);
    const unsubscribe = subscribeTags((loaded) => {
      if (alive) setAllTags(loaded);
    });
    return () => {
      alive = false;
      unsubscribe();
    };
  }, []);

  // Refresh once per missing ID: it may have been created after the cached list loaded.
  // Remember missing IDs to avoid refetch loops for deleted tags.
  const refreshedFor = useRef(new Set<number>());
  useEffect(() => {
    if (!revalidated || allTags === undefined) return;
    const known = new Set(allTags.map((tag) => tag.id));
    const missing = tagIds.filter(
      (id) => !known.has(id) && !refreshedFor.current.has(id),
    );
    if (missing.length === 0) return;
    for (const id of missing) refreshedFor.current.add(id);
    void refreshTags().catch(() => undefined);
  }, [allTags, revalidated, tagIds]);

  return new Map((allTags ?? []).map((tag) => [tag.id, tag]));
}

/** Return undefined while loading; only a 404 calls onGone, while other failures keep the skeleton. */
export function useCreditName(
  kind: CreditKind,
  id: number,
  onGone: (kind: CreditKind) => void,
): string | undefined {
  const [loaded, setLoaded] = useState<{ kind: CreditKind; id: number; name: string }>();
  const gone = useEffectEvent(() => onGone(kind));
  useEffect(() => {
    const controller = new AbortController();
    getCredit(kind, id, controller.signal)
      .then((credit) => setLoaded({ kind, id, name: credit.name }))
      .catch((failure: unknown) => {
        if (controller.signal.aborted) return;
        if (failure instanceof RequestFailed && failure.status === 404) gone();
      });
    return () => controller.abort();
  }, [kind, id]);
  return loaded?.kind === kind && loaded.id === id ? loaded.name : undefined;
}
