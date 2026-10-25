import { useEffect, useReducer } from "react";

import {
  listWorkInfoFillTargets,
  searchWorkInfoMatch,
  type WorkInfoCandidate,
  type WorkInfoFillItem,
  type WorkInfoFillTarget,
} from "../../api/workInfoPlugins";
import { errorText, type UiText } from "../../i18n";

export interface FillTargets {
  videoIds: readonly number[];
  smartFolderIds: readonly number[];
}

export type FillRowStatus = "waiting" | "searching" | "done" | "failed";

/** One video under review: its search result and the candidate chosen for it, or null to leave it alone. */
export interface FillRow {
  target: WorkInfoFillTarget;
  status: FillRowStatus;
  candidates: WorkInfoCandidate[];
  choice: number | null;
  failure: UiText | null;
}

interface ReviewState {
  rows: FillRow[] | null;
  failure: UiText | null;
}

type Action =
  | { type: "listed"; targets: WorkInfoFillTarget[] }
  | { type: "listFailed"; failure: UiText }
  | { type: "row"; index: number; row: Partial<FillRow> }
  | { type: "choose"; index: number; choice: number | null };

function reduce(state: ReviewState, action: Action): ReviewState {
  switch (action.type) {
    case "listed":
      return {
        failure: null,
        rows: action.targets.map((target) => ({
          target,
          status: "waiting",
          candidates: [],
          choice: null,
          failure: null,
        })),
      };
    case "listFailed":
      return { rows: null, failure: action.failure };
    case "row":
    case "choose": {
      const patch = action.type === "row" ? action.row : { choice: action.choice };
      const rows = state.rows?.map((row, i) =>
        i === action.index ? { ...row, ...patch } : row,
      );
      return { ...state, rows: rows ?? null };
    }
  }
}

// Searches run one at a time, since the plugin runtime has a single run slot.
async function searchAll(
  pluginId: number,
  targets: WorkInfoFillTarget[],
  dispatch: (action: Action) => void,
  signal: AbortSignal,
) {
  for (const [index, target] of targets.entries()) {
    dispatch({ type: "row", index, row: { status: "searching" } });
    try {
      const result = await searchWorkInfoMatch(pluginId, target.query, signal);
      const row = {
        status: "done" as const,
        candidates: result.candidates,
        choice: result.match ?? null,
      };
      dispatch({ type: "row", index, row });
    } catch (error) {
      if (signal.aborted) return;
      dispatch({
        type: "row",
        index,
        row: { status: "failed", failure: errorText(error) },
      });
    }
  }
}

/**
 * List the targets, then search for each with the plugin and preselect its match. Leaving the review aborts the
 * remaining searches.
 */
export function useFillReview(pluginId: number, targets: FillTargets) {
  const [state, dispatch] = useReducer(reduce, { rows: null, failure: null });
  useEffect(() => {
    const controller = new AbortController();
    const { signal } = controller;
    listWorkInfoFillTargets(targets.videoIds, targets.smartFolderIds, signal).then(
      (listed) => {
        dispatch({ type: "listed", targets: listed });
        void searchAll(pluginId, listed, dispatch, signal);
      },
      (error: unknown) => {
        if (!signal.aborted) dispatch({ type: "listFailed", failure: errorText(error) });
      },
    );
    return () => controller.abort();
  }, [pluginId, targets]);
  return {
    ...state,
    choose: (index: number, choice: number | null) =>
      dispatch({ type: "choose", index, choice }),
  };
}

/** The fills the owner confirmed: every row with a chosen candidate. */
export function chosenItems(rows: readonly FillRow[]): WorkInfoFillItem[] {
  return rows.flatMap((row) => {
    const chosen = row.choice === null ? undefined : row.candidates[row.choice];
    return chosen === undefined
      ? []
      : [{ videoId: row.target.videoId, workId: chosen.id }];
  });
}
