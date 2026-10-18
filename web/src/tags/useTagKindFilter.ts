import { useEffect, useLayoutEffect, useRef } from "react";

import type { TagKind } from "../api/tagKinds";
import type { TagKindFilter } from "./tagListUrl";

export function kindNameOf(
  kinds: readonly TagKind[] | undefined,
  kind: TagKindFilter | undefined,
): string | undefined {
  if (kind === undefined || kind === null || kind === "none") return undefined;
  return kinds?.find((item) => String(item.id) === kind)?.name;
}

/** Remove missing kind filters only after a successful read since mount, never from stale cache or failures. */
export function isGoneKind(
  kind: TagKindFilter,
  kinds: readonly TagKind[] | undefined,
  fresh: boolean,
): boolean {
  if (!fresh || kinds === undefined || kind === null || kind === "none") return false;
  return !kinds.some((item) => String(item.id) === kind);
}

export function useTagKindFilter({
  kind,
  kinds,
  fresh,
  onGone,
}: {
  kind: TagKindFilter;
  kinds: readonly TagKind[] | undefined;
  fresh: boolean;
  onGone: () => void;
}): { kindName: string | undefined } {
  const onGoneRef = useRef(onGone);
  useLayoutEffect(() => {
    onGoneRef.current = onGone;
  });
  const gone = isGoneKind(kind, kinds, fresh);
  useEffect(() => {
    if (gone) onGoneRef.current();
  }, [gone, kind]);
  return { kindName: kindNameOf(kinds, kind) };
}
