import { useCallback, useEffect } from "react";

import {
  addedLengthValues,
  type LengthRange,
  type ParsedAddedLength,
  withAddedWithin,
  withLength,
} from "./addedLengthCriteria";
import type { ApplyTagKind } from "./useLibraryTagKinds";

/** Replace invalid URL filters before requesting data; explicit changes push history and preserve other parameters. */
export function useAddedLengthControls(
  parsed: ParsedAddedLength,
  apply: ApplyTagKind,
): {
  changeAdded: (days: number | undefined) => void;
  changeLength: (range: LengthRange) => void;
} {
  const { filter, dropped } = parsed;
  const changeAdded = useCallback(
    (days: number | undefined) => apply(withAddedWithin(days), "push"),
    [apply],
  );
  const changeLength = useCallback(
    (range: LengthRange) => apply(withLength(range), "push"),
    [apply],
  );
  useEffect(() => {
    if (dropped) apply(addedLengthValues(filter), "replace");
  }, [filter, dropped, apply]);
  return { changeAdded, changeLength };
}
