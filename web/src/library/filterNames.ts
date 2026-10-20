import { useEffect, useEffectEvent, useRef, useState } from "react";

import { RequestFailed } from "../api/client";
import { type CreditKind, getCredit } from "../api/credits";
import { creditDisplayName } from "../credits/displayName";
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

/** A credit's display name and face revision, as filter chips show it. */
export interface ShownCredit {
  id: number;
  name: string;
  imageRevision?: number;
}

/** Return undefined while loading; only a 404 calls onGone, while other failures keep the skeleton. */
export function useCreditRef(
  kind: CreditKind,
  id: number,
  onGone: (kind: CreditKind) => void,
): ShownCredit | undefined {
  const [loaded, setLoaded] = useState<{ kind: CreditKind; ref: ShownCredit }>();
  const gone = useEffectEvent(() => onGone(kind));
  useEffect(() => {
    const controller = new AbortController();
    getCredit(kind, id, controller.signal)
      .then((credit) =>
        setLoaded({
          kind,
          ref: {
            id,
            name: creditDisplayName(credit),
            ...("imageRevision" in credit && { imageRevision: credit.imageRevision }),
          },
        }),
      )
      .catch((failure: unknown) => {
        if (controller.signal.aborted) return;
        if (failure instanceof RequestFailed && failure.status === 404) gone();
      });
    return () => controller.abort();
  }, [kind, id]);
  return loaded?.kind === kind && loaded.ref.id === id ? loaded.ref : undefined;
}

export function useCreditName(
  kind: CreditKind,
  id: number,
  onGone: (kind: CreditKind) => void,
): string | undefined {
  return useCreditRef(kind, id, onGone)?.name;
}
