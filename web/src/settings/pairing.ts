import { RequestFailed } from "../api/client";
import { errorText, t, type UiText } from "../i18n";

/** The server compares the code as one uppercase run, so drop the space or hyphen between groups. */
export function normalizePairingCode(input: string): string {
  return input.replace(/[\s-]+/g, "").toUpperCase();
}

/** Two groups of four, as the Pairing page shows the code. */
export function groupPairingCode(code: string): string {
  return code.length === 8 ? `${code.slice(0, 4)} ${code.slice(4)}` : code;
}

/** m:ss of the whole seconds left, rounded up so a new code starts at 10:00. */
export function formatCountdown(remainingMs: number): string {
  const total = Math.max(0, Math.ceil(remainingMs / 1000));
  const seconds = total % 60;
  return `${String(Math.floor(total / 60))}:${String(seconds).padStart(2, "0")}`;
}

export type PairField = "address" | "code" | "ownAddress";

export type PairFailure =
  | { kind: "field"; field: PairField; text: UiText }
  | { kind: "alert"; text: UiText }
  | { kind: "untrusted" | "ownUntrusted"; fingerprint: string };

function addressField(field: string | undefined): PairField {
  return field === "ownAddress" ? "ownAddress" : "address";
}

/** Where a refused pairing shows its reason: under a field, in an alert, or as a fingerprint to trust. */
export function pairFailure(failure: unknown): PairFailure {
  const text = t.settings.pairing;
  if (!(failure instanceof RequestFailed)) {
    return { kind: "alert", text: text.form.failed(errorText(failure)) };
  }
  switch (failure.reason) {
    case "invalid_address":
      return {
        kind: "field",
        field: addressField(failure.field),
        text: text.invalidAddress,
      };
    case "invalid_code":
      return { kind: "field", field: "code", text: text.form.invalidCode };
    case "already_paired":
      return { kind: "alert", text: text.form.alreadyPaired };
    case "peer_unreachable":
      // The server's message says which direction failed; the Words table shows it as given.
      if (failure.message !== "")
        return { kind: "alert", text: text.form.unreachable(failure.message) };
      break;
    case "certificate_untrusted":
      if (failure.fingerprint !== undefined)
        return { kind: "untrusted", fingerprint: failure.fingerprint };
      break;
    case "own_certificate_unverified":
      if (failure.fingerprint !== undefined)
        return { kind: "ownUntrusted", fingerprint: failure.fingerprint };
      break;
  }
  return { kind: "alert", text: text.form.failed(errorText(failure)) };
}
