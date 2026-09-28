import {
  NetworkFailed,
  RequestFailed,
  type ProbeErrorCode,
  type Scan,
  type ScanErrorCode,
} from "../api/client";
import type { ErrorDetails } from "./en";
import { t } from "./messages";
import { asUiText, type UiText } from "./uiText";

// 失敗を画面の英語の文言にする（specs/023-english-i18n/research.md R-5・R-6）。
// サーバーの自由文（ffprobe の出力、過去の日本語の理由）は出さない。

type Entry = UiText | ((details: ErrorDetails) => UiText);

function lookup(table: object, key: string | undefined): Entry | undefined {
  if (key === undefined || !Object.hasOwn(table, key)) return undefined;
  return (table as Record<string, Entry>)[key];
}

function render(entry: Entry, details: ErrorDetails): UiText {
  return typeof entry === "function" ? entry(details) : entry;
}

/**
 * errorText は失敗を画面に出す英語の文言にする。順は次のとおり。
 *
 * 1. 既知の `reason`、なければ既知の `code`: カタログの文（`limit`・`tagName` を埋め込む）
 * 2. 未知の `code` で `message` がある: サーバーの英語の `message`
 * 3. 本文が JSON でない・空・`message` が無い: `Request failed (HTTP <status>)`
 * 4. fetch 自体の失敗: サーバーに届かなかったことを表す概要（ブラウザの文言は出さない）
 *
 * 操作ごとの文脈（フォルダ名かタグ名か）は呼び出し側の文言が囲む。
 */
export function errorText(error: unknown): UiText {
  if (error instanceof RequestFailed) {
    const details: ErrorDetails = { limit: error.limit, tagName: error.tagName };
    const entry =
      lookup(t.errors.reason, error.reason) ?? lookup(t.errors.code, error.code);
    if (entry !== undefined) return render(entry, details);
    // サーバーの message は英語の文である（contracts/error-api.md §0）。未知のコードの
    // フォールバックとしてだけ出す。
    if (error.code !== "" && error.message !== "") return asUiText(error.message);
    return t.errors.requestFailed(error.status);
  }
  if (error instanceof NetworkFailed) return t.errors.unreachable;
  return t.errors.unexpected;
}

/**
 * probeErrorText は解析の失敗の説明である。`probeError`（自由文）は使わない。コードの
 * 無い行（英語化の前の失敗）は一般的な概要を出す。
 */
export function probeErrorText(code: ProbeErrorCode | undefined): UiText {
  const entry = lookup(t.errors.probe, code);
  return typeof entry === "string" ? entry : t.errors.probeUnknown;
}

/**
 * scanErrorText は取り込みの失敗の説明である。`Scan.error`（自由文）は使わず、
 * `errorCode` と `errorPath` から作る。コードの無い行は一般的な概要を出す。
 */
export function scanErrorText(scan: Pick<Scan, "errorCode" | "errorPath">): UiText {
  const code: ScanErrorCode | undefined = scan.errorCode;
  if (code === undefined || !Object.hasOwn(t.errors.scan, code))
    return t.errors.scanUnknown;
  return t.errors.scan[code](scan.errorPath);
}
