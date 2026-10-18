import { useCallback, useEffect, useEffectEvent, useRef, useState } from "react";

import {
  currentTagKinds,
  getTagKinds,
  refreshTagKinds,
  subscribeTagKinds,
  type TagKind,
} from "../api/tagKinds";
import { t } from "../i18n";
import { useToast } from "../ui/Toast";
import type { HistoryMode } from "../videoList/listCriteria";
import { withoutTagKind, withTagKind } from "./creditCriteria";

export type ApplyTagKind = (
  extra: Readonly<Record<string, string[]>>,
  mode: HistoryMode,
) => void;

export interface LibraryTagKinds {
  kinds: TagKind[] | undefined;
  name: string | undefined;
  load: () => void;
  change: (id: number | undefined) => void;
}

const ignore = () => undefined;

/** Load lazily unless a filter is active. Revalidate stale cache before declaring a kind missing; failures never remove filters, and opening retries. */
export function useLibraryTagKinds(
  tagKind: number | undefined,
  apply: ApplyTagKind,
): LibraryTagKinds {
  const toast = useToast();
  const [kinds, setKinds] = useState<TagKind[] | undefined>(currentTagKinds);
  // Retain the initial array reference to distinguish stale cache from revalidated data.
  const [cachedAtMount] = useState<TagKind[] | undefined>(currentTagKinds);

  useEffect(() => subscribeTagKinds(setKinds), []);

  const unconfirmed =
    tagKind !== undefined &&
    kinds !== undefined &&
    kinds === cachedAtMount &&
    !kinds.some((kind) => kind.id === tagKind);

  // getTagKinds may reuse stale cache; opening explicitly refreshes, including after failures.
  const load = useCallback(() => {
    if (unconfirmed) refreshTagKinds().catch(ignore);
    else getTagKinds().then(setKinds).catch(ignore);
  }, [unconfirmed]);

  const active = tagKind !== undefined;
  const missingList = kinds === undefined;
  useEffect(() => {
    if (active && missingList) load();
  }, [active, missingList, load]);

  const gone = useEffectEvent(() => {
    toast(t.library.activeTags.kindGone);
    apply(withoutTagKind, "replace");
  });
  // Deduplicate refreshes and removals across StrictMode and repeated renders.
  // Forget removed filters so revisiting them performs another existence check.
  const handled = useRef<number | undefined>(undefined);
  const refreshed = useRef<number | undefined>(undefined);
  useEffect(() => {
    if (tagKind === undefined) {
      handled.current = undefined;
      refreshed.current = undefined;
      return;
    }
    if (kinds === undefined || kinds.some((kind) => kind.id === tagKind)) return;
    if (kinds === cachedAtMount) {
      if (refreshed.current === tagKind) return;
      refreshed.current = tagKind;
      refreshTagKinds().catch(ignore);
      return;
    }
    if (handled.current === tagKind) return;
    handled.current = tagKind;
    gone();
  }, [cachedAtMount, kinds, tagKind]);

  const change = useCallback(
    (id: number | undefined) =>
      apply(id === undefined ? withoutTagKind : withTagKind(id), "push"),
    [apply],
  );

  const name =
    tagKind === undefined ? undefined : kinds?.find((kind) => kind.id === tagKind)?.name;
  return { kinds, name, load, change };
}
