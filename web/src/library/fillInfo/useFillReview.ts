import { useEffect, useReducer, useRef } from "react";

import { getVideo } from "../../api/client";
import {
  applyWorkInfoFetch,
  fetchWorkInfo,
  listWorkInfoFillTargets,
  searchWorkInfoMatch,
  type WorkInfoCandidate,
  type WorkInfoFillTarget,
} from "../../api/workInfoPlugins";
import { errorText, type UiText } from "../../i18n";
import { workChoice, workSelectionEmpty } from "../../player/workInfoFetch/workSelection";
import { type FillPlan, fillPlan } from "./fillPlan";

export interface FillTargets {
  videoIds: readonly number[];
  smartFolderIds: readonly number[];
}

/**
 * Where a row stands: searched rows wait for the owner's choice, and a chosen work is fetched into a plan of what
 * the fill writes before it can be applied.
 */
export type FillRowStatus =
  "waiting" | "searching" | "searched" | "fetching" | "applying" | "applied" | "failed";

export interface FillRow {
  target: WorkInfoFillTarget;
  status: FillRowStatus;
  candidates: WorkInfoCandidate[];
  choice: number | null;
  /** The fetched work's plan, for the work it was fetched for. */
  plan: { workId: string; fetchId: string; plan: FillPlan } | null;
  failure: UiText | null;
}

interface ReviewState {
  rows: FillRow[] | null;
  failure: UiText | null;
  applying: boolean;
}

type Action =
  | { type: "listed"; targets: WorkInfoFillTarget[] }
  | { type: "listFailed"; failure: UiText }
  | { type: "row"; index: number; row: Partial<FillRow> }
  | { type: "applying"; applying: boolean };

function newRow(target: WorkInfoFillTarget): FillRow {
  return {
    target,
    status: "waiting",
    candidates: [],
    choice: null,
    plan: null,
    failure: null,
  };
}

function reduce(state: ReviewState, action: Action): ReviewState {
  switch (action.type) {
    case "listed":
      return { ...state, failure: null, rows: action.targets.map(newRow) };
    case "listFailed":
      return { ...state, rows: null, failure: action.failure };
    case "applying":
      return { ...state, applying: action.applying };
    case "row": {
      const rows = state.rows?.map((row, i) =>
        i === action.index ? { ...row, ...action.row } : row,
      );
      return { ...state, rows: rows ?? null };
    }
  }
}

export function chosenWork(row: FillRow): WorkInfoCandidate | undefined {
  return row.choice === null ? undefined : row.candidates[row.choice];
}

/** A row the apply writes: its chosen work is fetched and the plan changes something. */
export function applicable(row: FillRow): boolean {
  const work = chosenWork(row);
  return (
    row.status === "searched" &&
    work !== undefined &&
    row.plan?.workId === work.id &&
    !workSelectionEmpty(row.plan.plan.selection)
  );
}

function needsFetch(row: FillRow): boolean {
  const work = chosenWork(row);
  return row.status === "searched" && work !== undefined && row.plan?.workId !== work.id;
}

// Searches and fetches run one at a time, since the plugin runtime has a single run slot.
async function runStep(
  pluginId: number,
  rows: FillRow[],
  index: number,
  dispatch: (action: Action) => void,
  signal: AbortSignal,
) {
  const row = rows[index];
  if (row === undefined) return;
  try {
    if (row.status === "waiting") {
      dispatch({ type: "row", index, row: { status: "searching" } });
      const result = await searchWorkInfoMatch(pluginId, row.target.query, signal);
      const choice = result.match ?? null;
      dispatch({
        type: "row",
        index,
        row: { status: "searched", candidates: result.candidates, choice },
      });
      return;
    }
    const work = chosenWork(row);
    if (work === undefined) return;
    dispatch({ type: "row", index, row: { status: "fetching" } });
    const [video, fetched] = await Promise.all([
      getVideo(row.target.videoId, signal),
      fetchWorkInfo(row.target.videoId, pluginId, work.id, signal),
    ]);
    const plan = {
      workId: work.id,
      fetchId: fetched.fetchId,
      plan: fillPlan(video, fetched),
    };
    dispatch({ type: "row", index, row: { status: "searched", plan } });
  } catch (error) {
    if (signal.aborted) return;
    dispatch({
      type: "row",
      index,
      row: { status: "failed", failure: errorText(error) },
    });
  }
}

async function applyAll(
  rows: FillRow[],
  dispatch: (action: Action) => void,
): Promise<number> {
  let applied = 0;
  dispatch({ type: "applying", applying: true });
  for (const [index, row] of rows.entries()) {
    if (!applicable(row) || row.plan === null) continue;
    dispatch({ type: "row", index, row: { status: "applying" } });
    try {
      await applyWorkInfoFetch(
        row.target.videoId,
        row.plan.fetchId,
        workChoice(row.plan.plan.selection),
      );
      dispatch({ type: "row", index, row: { status: "applied" } });
      applied++;
    } catch (error) {
      dispatch({
        type: "row",
        index,
        row: { status: "failed", failure: errorText(error) },
      });
    }
  }
  dispatch({ type: "applying", applying: false });
  return applied;
}

/**
 * List the targets, search for each, and fetch each chosen work into a plan of what the fill writes. Leaving the
 * review aborts the remaining runs; the apply writes the fetched results the review showed, one video at a time.
 */
export function useFillReview(pluginId: number, targets: FillTargets) {
  const [state, dispatch] = useReducer(reduce, {
    rows: null,
    failure: null,
    applying: false,
  });
  const controller = useRef<AbortController | null>(null);
  const busy = useRef(false);

  useEffect(() => {
    const current = new AbortController();
    controller.current = current;
    listWorkInfoFillTargets(
      targets.videoIds,
      targets.smartFolderIds,
      current.signal,
    ).then(
      (listed) => dispatch({ type: "listed", targets: listed }),
      (error: unknown) => {
        if (!current.signal.aborted)
          dispatch({ type: "listFailed", failure: errorText(error) });
      },
    );
    return () => current.abort();
  }, [targets]);

  useEffect(() => {
    const rows = state.rows;
    const signal = controller.current?.signal;
    if (rows === null || signal === undefined || busy.current || state.applying) return;
    const index = rows.findIndex((row) => row.status === "waiting" || needsFetch(row));
    if (index < 0) return;
    busy.current = true;
    void runStep(pluginId, rows, index, dispatch, signal).finally(() => {
      busy.current = false;
      dispatch({ type: "row", index, row: {} });
    });
  }, [state.rows, state.applying, pluginId]);

  return {
    ...state,
    choose: (index: number, choice: number | null) =>
      dispatch({
        type: "row",
        index,
        row: { choice, failure: null, status: "searched" },
      }),
    apply: () => applyAll(state.rows ?? [], dispatch),
  };
}
