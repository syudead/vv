import type { ThumbnailEdit, ThumbnailEditFailure } from "../../api/client";
import { t, type UiText } from "../../i18n";
import { isRunning } from "./editRules";

export type EditorView = "compose" | "running" | "compare";

export interface RunFailure {
  title: UiText;
  message?: string;
}

export interface EditorState {
  session: ThumbnailEdit;
  receivedAt: number;
  view: EditorView;
  failure: RunFailure | null;
  cancelled: boolean;
  ran: boolean;
  /** Never restore an old result after another run; failed or cancelled sessions cannot apply it. */
  resultGone: boolean;
  /** Ended sessions allow only closing. */
  ended: boolean;
}

export type EditorAction =
  | { type: "read"; session: ThumbnailEdit; at: number }
  | { type: "started"; session: ThumbnailEdit; at: number }
  | { type: "rejected"; failure: RunFailure }
  | { type: "change" }
  | { type: "showResult" }
  | { type: "ended" };

export function initialEditorState(session: ThumbnailEdit, at: number): EditorState {
  return {
    session,
    receivedAt: at,
    view: "compose",
    failure: null,
    cancelled: false,
    ran: false,
    resultGone: false,
    ended: false,
  };
}

export function failureReasonText(reason: ThumbnailEditFailure | undefined): UiText {
  if (reason === undefined) return t.errors.unexpected;
  return t.errors.reason[reason];
}

export function hasResult(state: EditorState): boolean {
  return (
    !state.ended &&
    !state.resultGone &&
    state.session.state === "succeeded" &&
    state.session.resultUrl !== undefined
  );
}

/** Confirm before losing an unapplied result or an active run that consumes time and cloud credits. */
export function needsConfirmation(state: EditorState): boolean {
  if (state.ended) return false;
  return isRunning(state.session.state) || hasResult(state);
}

function read(state: EditorState, session: ThumbnailEdit, at: number): EditorState {
  const next = { ...state, session, receivedAt: at };
  if (isRunning(session.state)) return { ...next, view: "running" };
  // Heartbeat polls outside Running must not change the editor stage.
  if (state.view !== "running") return next;
  switch (session.state) {
    case "succeeded":
      return { ...next, view: "compare", resultGone: false };
    case "failed":
      return {
        ...next,
        view: "compose",
        failure: {
          title: t.player.thumbnailEdit.runFailed(
            failureReasonText(session.error?.reason),
          ),
          message: session.error?.message,
        },
      };
    case "cancelled":
      return { ...next, view: "compose", cancelled: true };
    default:
      return { ...next, view: "compose" };
  }
}

export function editorReducer(state: EditorState, action: EditorAction): EditorState {
  if (state.ended && action.type !== "ended") return state;
  switch (action.type) {
    case "read":
      return read(state, action.session, action.at);
    case "started":
      return read(
        {
          ...state,
          view: "running",
          failure: null,
          cancelled: false,
          ran: true,
          resultGone: true,
        },
        action.session,
        action.at,
      );
    case "rejected":
      return {
        ...state,
        view: "compose",
        failure: action.failure,
        cancelled: false,
        ran: true,
        resultGone: true,
      };
    case "change":
      return state.view === "compare" ? { ...state, view: "compose" } : state;
    case "showResult":
      return state.view === "compose" && hasResult(state)
        ? { ...state, view: "compare" }
        : state;
    case "ended":
      return { ...state, ended: true };
  }
}
