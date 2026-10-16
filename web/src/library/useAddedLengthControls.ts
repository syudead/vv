import { useCallback, useEffect } from "react";

import {
  addedLengthValues,
  type LengthRange,
  type ParsedAddedLength,
  withAddedWithin,
  withLength,
} from "./addedLengthCriteria";
import type { ApplyTagKind } from "./useLibraryTagKinds";

/**
 * useAddedLengthControls はライブラリの追加日と長さの絞り込みを URL へ書く操作を返す
 * （specs/052-smart-folders/ui-design.md「Added and length」）。選ぶとその値だけを書き換えて
 * 履歴を 1 つ増やし、ほかの条件は残る。URL の送れない値（範囲の外、下限が上限を超える、
 * parsed.dropped）は、要求の前に履歴を増やさずに URL から落とす。
 */
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
