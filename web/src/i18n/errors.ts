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

// Never display raw server diagnostics such as ffprobe output or legacy localized errors.

type Entry = UiText | ((details: ErrorDetails) => UiText);

function lookup(table: object, key: string | undefined): Entry | undefined {
  if (key === undefined || !Object.hasOwn(table, key)) return undefined;
  return (table as Record<string, Entry>)[key];
}

function render(entry: Entry, details: ErrorDetails): UiText {
  return typeof entry === "function" ? entry(details) : entry;
}

/**
 * Prefer known reason, then code, then an unknown code's server message, then HTTP status. Network
 * failures use generic copy; callers add operation context.
 */
export function errorText(error: unknown): UiText {
  if (error instanceof RequestFailed) {
    const details: ErrorDetails = {
      limit: error.limit,
      tagName: error.tagName,
      host: error.host,
    };
    const entry =
      lookup(t.errors.reason, error.reason) ?? lookup(t.errors.code, error.code);
    if (entry !== undefined) return render(entry, details);
    // Show server messages only as a fallback for unknown codes.
    if (error.code !== "" && error.message !== "") return asUiText(error.message);
    return t.errors.requestFailed(error.status);
  }
  if (error instanceof NetworkFailed) return t.errors.unreachable;
  return t.errors.unexpected;
}

/** Ignore freeform probeError; legacy records without a code get a generic summary. */
export function probeErrorText(code: ProbeErrorCode | undefined): UiText {
  const entry = lookup(t.errors.probe, code);
  return typeof entry === "string" ? entry : t.errors.probeUnknown;
}

/** Use errorCode and errorPath, never freeform Scan.error; missing codes get a generic summary. */
export function scanErrorText(scan: Pick<Scan, "errorCode" | "errorPath">): UiText {
  const code: ScanErrorCode | undefined = scan.errorCode;
  if (code === undefined || !Object.hasOwn(t.errors.scan, code))
    return t.errors.scanUnknown;
  return t.errors.scan[code](scan.errorPath);
}
