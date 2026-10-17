import type { ThumbnailEdit, ThumbnailEditFailure } from "../../api/client";
import { t, type UiText } from "../../i18n";
import { isRunning } from "./editRules";

// 編集画面の見え方の移り変わり。サーバーのセッションを読むたび、操作の応答を受けるたびに、どの見え方
// （Compose・Running・Compare）にするかをここで決める。

export type EditorView = "compose" | "running" | "compare";

/** RunFailure は実行の失敗の Alert の題と、提供元の文（あれば）である。 */
export interface RunFailure {
  title: UiText;
  message?: string;
}

export interface EditorState {
  session: ThumbnailEdit;
  /** session を受け取った時刻（ミリ秒）。経過時間をここから進める。 */
  receivedAt: number;
  view: EditorView;
  /** 最後の実行の失敗。次の実行を始めるまで出す。 */
  failure: RunFailure | null;
  /** 最後の実行が中止で終わった。 */
  cancelled: boolean;
  /** 実行を 1 度でも送った（主な操作が Generate again になる）。 */
  ran: boolean;
  /**
   * 前の結果を画面から外した。実行を送ったら、その結果は二度と出さない（適用できるのは
   * `succeeded` のセッションだけで、失敗や中止の後に適用できない画像を残さないため）。
   */
  resultGone: boolean;
  /** セッションが終わった（404 か `content_changed`）。閉じる × のほかは押せない。 */
  ended: boolean;
}

export type EditorAction =
  | { type: "read"; session: ThumbnailEdit; at: number }
  | { type: "started"; session: ThumbnailEdit; at: number }
  | { type: "rejected"; failure: RunFailure }
  | { type: "change" }
  | { type: "showResult" }
  | { type: "ended" };

/** initialEditorState は始めたばかりのセッションの見え方（Compose）である。 */
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

/** failureReasonText は実行が失敗した理由の文言である。 */
export function failureReasonText(reason: ThumbnailEditFailure | undefined): UiText {
  if (reason === undefined) return t.errors.unexpected;
  return t.errors.reason[reason];
}

/**
 * hasResult は Compare を出せる結果があるかを返す。セッションが `succeeded` で、その後に
 * 実行を送っていないときである。
 */
export function hasResult(state: EditorState): boolean {
  return (
    !state.ended &&
    !state.resultGone &&
    state.session.state === "succeeded" &&
    state.session.resultUrl !== undefined
  );
}

/**
 * needsConfirmation は閉じる前に確かめるかを返す。実行が進んでいるか、適用していない結果が
 * あるときで、生成した画像は取り戻せず、実行は時間と（Comfy Cloud では）クレジットを使う。
 */
export function needsConfirmation(state: EditorState): boolean {
  if (state.ended) return false;
  return isRunning(state.session.state) || hasResult(state);
}

function read(state: EditorState, session: ThumbnailEdit, at: number): EditorState {
  const next = { ...state, session, receivedAt: at };
  if (isRunning(session.state)) return { ...next, view: "running" };
  // 実行を見守っていないとき（Compose・Compare）の読み取りは、見え方を変えない。
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
