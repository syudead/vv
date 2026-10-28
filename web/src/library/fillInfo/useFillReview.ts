import { useEffect, useReducer, useRef } from "react";

import { getVideo, type Video } from "../../api/client";
import {
  applyWorkInfoFetch,
  fetchWorkInfo,
  listWorkInfoFillTargets,
  searchWorkInfoMatch,
  type WorkInfoCandidate,
  type WorkInfoFetch,
  type WorkInfoFillTarget,
} from "../../api/workInfoPlugins";
import { errorText, t, type UiText } from "../../i18n";
import { isNarrowVideo } from "../../lib/format";
import type { UploadImage } from "../../player/thumbnailUpload/composeImage";
import { type Fit, frameAspect, WHOLE_IMAGE } from "../../player/thumbnailUpload/framing";
import {
  type FramedCover,
  loadFetchedCover,
  uploadFramedCover,
} from "../../player/workInfoFetch/coverFraming";
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

/** The cover a plan fills, which the page frames as an uploaded image: the whole image over its blurred copy unless the owner adjusts it. */
export interface PlannedCover {
  index: number;
  label: string;
  size: string;
  aspect: number | null;
  narrow: boolean;
}

export interface FillRow {
  target: WorkInfoFillTarget;
  status: FillRowStatus;
  candidates: WorkInfoCandidate[];
  choice: number | null;
  /** The fetched work's plan, for the work it was fetched for. */
  plan: {
    workId: string;
    fetchId: string;
    plan: FillPlan;
    cover: PlannedCover | null;
  } | null;
  /** The owner's framing of the plan's cover, with the decoded image it frames. */
  framing: FramedCover | null;
  failure: UiText | null;
}

interface ReviewState {
  rows: FillRow[] | null;
  failure: UiText | null;
  applying: boolean;
  /** The row whose cover the framing dialog shows, while its cover is decoded and after. */
  adjusting: { index: number; ready: boolean } | null;
}

type Action =
  | { type: "listed"; targets: WorkInfoFillTarget[] }
  | { type: "listFailed"; failure: UiText }
  | { type: "row"; index: number; row: Partial<FillRow> }
  | { type: "applying"; applying: boolean }
  | { type: "adjusting"; adjusting: ReviewState["adjusting"] };

function newRow(target: WorkInfoFillTarget): FillRow {
  return {
    target,
    status: "waiting",
    candidates: [],
    choice: null,
    plan: null,
    framing: null,
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
    case "adjusting":
      return { ...state, adjusting: action.adjusting };
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

function plannedCover(video: Video, result: WorkInfoFetch, index: number | null) {
  const size = index === null ? undefined : result.covers[index];
  if (index === null || size === undefined) return null;
  const words = t.player.workInfoFetch;
  return {
    index,
    label: words.fetchedCover(size.width, size.height),
    size: words.coverSize(size.width, size.height),
    aspect: frameAspect(video),
    narrow: isNarrowVideo(video),
  };
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
    const filled = fillPlan(video, fetched);
    const plan = {
      workId: work.id,
      fetchId: fetched.fetchId,
      plan: filled,
      cover: plannedCover(video, fetched, filled.selection.cover),
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

/**
 * Apply one plan; a cover the page decodes is consumed by the apply and uploaded with the owner's framing or the
 * default one, and a cover it cannot decode is fitted by the server. Resolves whether the cover, when planned, was
 * saved.
 */
async function applyRow(
  videoId: number,
  plan: NonNullable<FillRow["plan"]>,
  framing: FramedCover | null,
) {
  const { cover } = plan;
  const image =
    cover === null
      ? null
      : (framing?.image ??
        (await loadFetchedCover(videoId, plan.fetchId, cover.index).catch(() => null)));
  try {
    let applied = await applyWorkInfoFetch(
      videoId,
      plan.fetchId,
      workChoice(plan.plan.selection),
      undefined,
      image !== null,
    );
    if (image !== null && cover !== null)
      applied = await uploadFramedCover(
        applied,
        { image, aspect: cover.aspect, fit: framing?.fit ?? WHOLE_IMAGE },
        cover.size,
      );
    return !applied.skipped.some((value) => value.field === "cover");
  } finally {
    if (image !== null && image !== framing?.image) image.bitmap.close();
  }
}

/** applied counts the videos written; complete is false when a row failed or its cover was not saved. */
async function applyAll(
  rows: FillRow[],
  dispatch: (action: Action) => void,
): Promise<{ applied: number; complete: boolean }> {
  let applied = 0;
  let complete = true;
  dispatch({ type: "applying", applying: true });
  for (const [index, row] of rows.entries()) {
    if (!applicable(row) || row.plan === null) continue;
    dispatch({ type: "row", index, row: { status: "applying" } });
    try {
      const coverSaved = await applyRow(row.target.videoId, row.plan, row.framing);
      const failure = coverSaved ? null : t.library.selection.fillReview.coverNotSaved;
      dispatch({ type: "row", index, row: { status: "applied", failure } });
      applied++;
      complete &&= coverSaved;
    } catch (error) {
      complete = false;
      dispatch({
        type: "row",
        index,
        row: { status: "failed", failure: errorText(error) },
      });
    }
  }
  dispatch({ type: "applying", applying: false });
  return { applied, complete };
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
    adjusting: null,
  });
  const controller = useRef<AbortController | null>(null);
  const busy = useRef(false);
  const framed = useFramedImages();

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

  const rowAt = (index: number) => state.rows?.[index];
  return {
    ...state,
    choose: (index: number, choice: number | null) => {
      framed.release(rowAt(index)?.framing?.image);
      dispatch({
        type: "row",
        index,
        row: { choice, failure: null, status: "searched", plan: null, framing: null },
      });
    },
    adjust: (index: number) => {
      const row = rowAt(index);
      if (row === undefined || state.adjusting !== null) return;
      void openFraming(row, index, dispatch, framed.hold, controller.current?.signal);
    },
    keepFraming: (index: number, fit: Fit, replaced: UploadImage | null) => {
      const framing = rowAt(index)?.framing;
      if (framing !== null && framing !== undefined) {
        let image = framing.image;
        if (replaced !== null) {
          framed.release(image);
          image = framed.hold(replaced);
        }
        dispatch({ type: "row", index, row: { framing: { ...framing, image, fit } } });
      }
      dispatch({ type: "adjusting", adjusting: null });
    },
    closeFraming: () => dispatch({ type: "adjusting", adjusting: null }),
    apply: () => applyAll(state.rows ?? [], dispatch),
  };
}

/** Decode the row's cover once, then show the framing dialog with the framing kept so far. */
async function openFraming(
  row: FillRow,
  index: number,
  dispatch: (action: Action) => void,
  hold: (image: UploadImage) => UploadImage,
  signal: AbortSignal | undefined,
) {
  const cover = row.plan?.cover;
  if (row.plan === null || cover === null || cover === undefined) return;
  if (row.framing !== null) {
    dispatch({ type: "adjusting", adjusting: { index, ready: true } });
    return;
  }
  dispatch({ type: "adjusting", adjusting: { index, ready: false } });
  const image = await loadFetchedCover(
    row.target.videoId,
    row.plan.fetchId,
    cover.index,
    signal,
  ).catch(() => null);
  if (signal?.aborted === true) {
    image?.bitmap.close();
    return;
  }
  if (image === null) {
    dispatch({ type: "adjusting", adjusting: null });
    return;
  }
  const framing = { image: hold(image), aspect: cover.aspect, fit: WHOLE_IMAGE };
  dispatch({ type: "row", index, row: { framing } });
  dispatch({ type: "adjusting", adjusting: { index, ready: true } });
}

/** Decoded covers the review holds for framing, closed when replaced or when the review leaves. */
function useFramedImages() {
  const held = useRef(new Set<UploadImage>());
  useEffect(() => {
    const images = held.current;
    return () => {
      for (const image of images) image.bitmap.close();
      images.clear();
    };
  }, []);
  return {
    hold: (image: UploadImage) => {
      held.current.add(image);
      return image;
    },
    release: (image: UploadImage | undefined) => {
      if (image === undefined || !held.current.delete(image)) return;
      image.bitmap.close();
    },
  };
}
