import { afterEach, describe, expect, it, vi } from "vitest";

import {
  type ErrorCode,
  type ErrorReason,
  NetworkFailed,
  type ProbeErrorCode,
  RequestFailed,
  request,
  type ScanErrorCode,
  toRequestFailed,
} from "../api/client";
import { errorText, probeErrorText, scanErrorText } from "./errors";
import { t } from "./messages";

// すべてのコードと理由に文があることは型で確かめる。生成型に値が増えて表に無ければ、
// ここ（と en.ts の satisfies）が型検査で落ちる。
const codeTable: Record<ErrorCode, unknown> = t.errors.code;
const reasonTable: Record<ErrorReason, unknown> = t.errors.reason;
const probeTable: Record<ProbeErrorCode, unknown> = t.errors.probe;
const scanTable: Record<ScanErrorCode, unknown> = t.errors.scan;

const japanese = /[぀-ヿ㐀-鿿]/;

function json(body: unknown, status: number): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

describe("error catalog", () => {
  it("has an English sentence for every code, reason and failure code", () => {
    const tables = [codeTable, reasonTable, probeTable];
    for (const table of tables) {
      for (const [key, entry] of Object.entries(table)) {
        const text =
          typeof entry === "function" ? (entry as (d: object) => string)({}) : entry;
        expect(typeof text, key).toBe("string");
        expect(text, key).not.toBe("");
        expect(text, key).not.toMatch(japanese);
      }
    }
    for (const [key, entry] of Object.entries(scanTable)) {
      const text = (entry as (path?: string) => string)("/media/a");
      expect(text, key).not.toBe("");
      expect(text, key).not.toMatch(japanese);
    }
  });
});

describe("errorText", () => {
  it("uses a known reason before the code and embeds the limit", async () => {
    const failure = await toRequestFailed(
      json(
        {
          code: "invalid_request",
          message: "Tag names must be at most 100 characters.",
          reason: "tag_name_too_long",
          limit: 100,
        },
        400,
      ),
    );
    expect(failure.status).toBe(400);
    expect(failure.code).toBe("invalid_request");
    expect(failure.reason).toBe("tag_name_too_long");
    expect(failure.limit).toBe(100);
    expect(errorText(failure)).toBe("Use a tag name of 100 characters or fewer.");
  });

  it("embeds a limit of one in the singular", () => {
    const failure = new RequestFailed(400, "invalid_request", "", {
      reason: "too_many_videos",
      limit: 1,
    });
    expect(errorText(failure)).toBe("Select between 1 and 1 video.");
  });

  it("embeds the conflicting tag name", async () => {
    const failure = await toRequestFailed(
      json(
        {
          code: "tag_name_taken",
          message: "…",
          reason: "name_is_synonym",
          tagName: "アニメ",
        },
        409,
      ),
    );
    expect(failure.tagName).toBe("アニメ");
    expect(errorText(failure)).toBe(
      'That name is already a synonym of the tag "アニメ".',
    );
  });

  it("uses the code when there is no reason, and ignores the server message", () => {
    expect(errorText(new RequestFailed(401, "invalid_credentials", "Wrong."))).toBe(
      "The username or password is incorrect.",
    );
  });

  it("falls back to the code when the reason is unknown", () => {
    const failure = new RequestFailed(404, "not_found", "Gone.", { reason: "brand_new" });
    expect(errorText(failure)).toBe("It wasn't found.");
  });

  it("shows the server's English message for an unknown code, keeping status and code", async () => {
    const failure = await toRequestFailed(
      json({ code: "quota_exceeded", message: "The quota is exceeded." }, 507),
    );
    expect(failure.status).toBe(507);
    expect(failure.code).toBe("quota_exceeded");
    expect(errorText(failure)).toBe("The quota is exceeded.");
  });

  it("summarizes an unknown code without a message by HTTP status", async () => {
    const failure = await toRequestFailed(json({ code: "quota_exceeded" }, 507));
    expect(failure.code).toBe("quota_exceeded");
    expect(errorText(failure)).toBe("Request failed (HTTP 507)");
  });

  it("summarizes an empty body by HTTP status", async () => {
    const failure = await toRequestFailed(new Response(null, { status: 502 }));
    expect(failure.status).toBe(502);
    expect(errorText(failure)).toBe("Request failed (HTTP 502)");
  });

  it("summarizes a body that is not JSON by HTTP status", async () => {
    const failure = await toRequestFailed(
      new Response("<html>Bad Gateway</html>", { status: 502 }),
    );
    expect(errorText(failure)).toBe("Request failed (HTTP 502)");
  });

  describe("when fetch itself fails", () => {
    afterEach(() => vi.unstubAllGlobals());

    it("says the server could not be reached, without the browser's text", async () => {
      vi.stubGlobal(
        "fetch",
        vi.fn(() =>
          Promise.reject(new TypeError("NetworkError when attempting to fetch")),
        ),
      );
      const failure: unknown = await request("/api/videos").catch(
        (error: unknown) => error,
      );
      expect(failure).toBeInstanceOf(NetworkFailed);
      expect(errorText(failure)).toBe(
        "Couldn't reach the server. Check that vv is running and try again.",
      );
    });

    it("lets an abort through unchanged", async () => {
      const abort = new DOMException("aborted", "AbortError");
      vi.stubGlobal(
        "fetch",
        vi.fn(() => Promise.reject(abort)),
      );
      await expect(request("/api/videos")).rejects.toBe(abort);
    });
  });

  it("does not show the text of other errors", () => {
    expect(errorText(new Error("x is undefined"))).toBe("Something went wrong.");
    expect(errorText("boom")).toBe("Something went wrong.");
  });
});

describe("probeErrorText and scanErrorText", () => {
  it("explains a probe failure from its code", () => {
    expect(probeErrorText("probe_failed")).toBe(
      "The file is damaged or isn't a supported video.",
    );
  });

  it("gives a general summary when there is no code", () => {
    expect(probeErrorText(undefined)).toBe("Couldn't read this video's information.");
    expect(scanErrorText({})).toBe("The scan failed.");
  });

  it("includes the path of a scan failure", () => {
    expect(
      scanErrorText({ errorCode: "media_folder_unreadable", errorPath: "/media/動画" }),
    ).toBe("The media folder couldn't be read: /media/動画");
    expect(scanErrorText({ errorCode: "interrupted" })).toBe("The scan was interrupted.");
  });
});
