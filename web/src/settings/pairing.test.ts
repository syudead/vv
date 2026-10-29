import { describe, expect, it } from "vitest";

import { NetworkFailed, RequestFailed } from "../api/client";
import {
  formatCountdown,
  groupPairingCode,
  normalizePairingCode,
  pairFailure,
} from "./pairing";

describe("pairing code", () => {
  it("sends the typed code as one uppercase run, whatever separates the groups", () => {
    expect(normalizePairingCode(" abcd efgh ")).toBe("ABCDEFGH");
    expect(normalizePairingCode("ABCD-EFGH")).toBe("ABCDEFGH");
  });

  it("shows the code as two groups of four", () => {
    expect(groupPairingCode("ABCDEFGH")).toBe("ABCD EFGH");
  });

  it("counts down in m:ss, starting at 10:00 and stopping at 0:00", () => {
    expect(formatCountdown(600_000)).toBe("10:00");
    expect(formatCountdown(598_001)).toBe("9:59");
    expect(formatCountdown(-5)).toBe("0:00");
  });
});

describe("pairFailure", () => {
  const conflict = (reason: string, details: object = {}, message = "") =>
    new RequestFailed(409, "conflict", message, { reason, ...details });

  it("puts an invalid address under the address field", () => {
    const failure = new RequestFailed(400, "invalid_request", "", {
      reason: "invalid_address",
      field: "address",
    });
    expect(pairFailure(failure)).toMatchObject({ kind: "field", field: "address" });
  });

  it("puts a refused code under the code field", () => {
    const failure = new RequestFailed(400, "invalid_request", "", {
      reason: "invalid_code",
    });
    expect(pairFailure(failure)).toMatchObject({ kind: "field", field: "code" });
  });

  it("asks to trust the fingerprint of an untrusted certificate", () => {
    const failure = conflict("certificate_untrusted", { fingerprint: "c2hh" });
    expect(pairFailure(failure)).toEqual({ kind: "untrusted", fingerprint: "c2hh" });
  });

  it("falls back to the reason's text when the details are missing", () => {
    expect(pairFailure(conflict("certificate_untrusted"))).toEqual({
      kind: "alert",
      text: "Couldn't complete the pairing: The other VVMDM uses a certificate this one can't verify.",
    });
    expect(pairFailure(new NetworkFailed(new Error("x")))).toMatchObject({
      kind: "alert",
    });
  });
});
