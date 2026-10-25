import { describe, expect, it } from "vitest";

import {
  applyItems,
  type CodeEditRow,
  type CompiledPattern,
  type DerivedRow,
  deriveRows,
  nextPattern,
  noPattern,
} from "./rows";

function row(id: number, fields: { fileTitle?: string; code?: string }): CodeEditRow {
  return {
    video: {
      id,
      virtual: fields.fileTitle === undefined,
      title: `Video ${id}`,
      fileTitle: fields.fileTitle,
      contentVersion: `v${id}`,
      sizeBytes: 0,
      addedAt: "2026-10-01T00:00:00Z",
      updatedAt: "2026-10-01T00:00:00Z",
      fileCreatedAt: "2026-10-01T00:00:00Z",
      fileModifiedAt: "2026-10-01T00:00:00Z",
      playable: true,
      probeState: "done",
      thumbnailState: "done",
      previewState: "done",
      tags: [],
      public: false,
    },
    code: fields.code,
  };
}

function compiled(pattern: string, replacement = ""): CompiledPattern {
  const state = nextPattern(pattern, replacement, noPattern);
  expect(state.error).toBeUndefined();
  return state.compiled;
}

function derive(
  rows: CodeEditRow[],
  options: {
    pattern?: CompiledPattern;
    upperCase?: boolean;
    typed?: [number, string][];
  } = {},
) {
  return deriveRows({
    rows,
    pattern: options.pattern ?? noPattern,
    upperCase: options.upperCase ?? false,
    typed: new Map(options.typed ?? []),
  });
}

function first(rows: DerivedRow[]): DerivedRow {
  const [result] = rows;
  if (result === undefined) throw new Error("no row");
  return result;
}

const values = (rows: ReturnType<typeof derive>) => rows.map((r) => [r.state, r.value]);

describe("deriveRows", () => {
  it("extracts the match into suggested rows only and keeps a non-matching file name", () => {
    const rows = [
      row(1, { fileTitle: "[xxx] ABC-123 1080p" }),
      row(2, { fileTitle: "[xxx] DEF-456", code: "KEEP-1" }),
      row(3, { fileTitle: "holiday" }),
      row(4, {}),
    ];
    expect(values(derive(rows, { pattern: compiled(String.raw`[A-Z]+-\d+`) }))).toEqual([
      ["suggested", "ABC-123"],
      ["existing", "KEEP-1"],
      ["suggested", "holiday"],
      ["blank", ""],
    ]);
  });

  it("takes group 1 with an empty replacement and substitutes $1 with a replacement", () => {
    const rows = [row(1, { fileTitle: "[xxx] ABC-123 1080p" })];
    const group = compiled(String.raw`\] (\w+)-(\d+)`);
    expect(first(derive(rows, { pattern: group })).value).toBe("ABC");
    const replaced = compiled(String.raw`^.*\] (\w+)-(\d+).*$`, "$1$2");
    expect(first(derive(rows, { pattern: replaced })).value).toBe("ABC123");
  });

  it("upper-cases suggested rows only", () => {
    const rows = [
      row(1, { fileTitle: "abc-123" }),
      row(2, { fileTitle: "x", code: "abc-1" }),
      row(3, { fileTitle: "y" }),
    ];
    expect(values(derive(rows, { upperCase: true, typed: [[3, "def-4"]] }))).toEqual([
      ["suggested", "ABC-123"],
      ["existing", "abc-1"],
      ["edited", "def-4"],
    ]);
  });

  it("keeps typed rows out of the pattern until the entry is removed", () => {
    const rows = [row(1, { fileTitle: "ABC-123 x" })];
    const pattern = compiled(String.raw`[A-Z]+-\d+`);
    expect(first(derive(rows, { pattern, typed: [[1, "  typed  "]] }))).toMatchObject({
      state: "edited",
      value: "typed",
    });
    expect(first(derive(rows, { pattern })).value).toBe("ABC-123");
  });

  it("marks every row sharing a non-empty value as a duplicate", () => {
    const rows = [
      row(1, { fileTitle: "ABC-1" }),
      row(2, { fileTitle: "y", code: "ABC-1" }),
      row(3, {}),
      row(4, {}),
      row(5, { fileTitle: "other" }),
    ];
    expect(derive(rows).map((r) => r.duplicate)).toEqual([
      true,
      true,
      false,
      false,
      false,
    ]);
  });

  it("cuts a value over 100 code points to 100 and flags it", () => {
    const long = "𝒜".repeat(101);
    const result = first(derive([row(1, { fileTitle: long })]));
    expect(result.cut).toBe(true);
    expect(Array.from(result.value)).toHaveLength(100);
    expect(first(derive([row(2, { fileTitle: "𝒜".repeat(100) })])).cut).toBe(false);
  });
});

describe("nextPattern", () => {
  it("reports an invalid pattern and keeps the last valid one", () => {
    const last = compiled(String.raw`[A-Z]+-\d+`);
    const state = nextPattern("[A-Z", "", last);
    expect(state.error).toEqual(expect.any(String));
    expect(state.compiled).toBe(last);
    expect(
      first(derive([row(1, { fileTitle: "x ABC-1" })], { pattern: state.compiled }))
        .value,
    ).toBe("ABC-1");
  });
});

describe("applyItems", () => {
  it("sends non-empty suggested and changed edited rows with their contentVersion", () => {
    const rows = [
      row(1, { fileTitle: "ABC-1" }),
      row(2, { fileTitle: "x", code: "KEEP" }),
      row(3, { fileTitle: "x", code: "OLD" }),
      row(4, { fileTitle: "x", code: "SAME" }),
      row(5, { fileTitle: "x" }),
      row(6, {}),
      row(7, {}),
    ];
    const typed: [number, string][] = [
      [3, "NEW"],
      [4, " SAME "],
      [5, "   "],
      [7, "V-1"],
    ];
    expect(applyItems(derive(rows, { typed }))).toEqual([
      { videoId: 1, contentVersion: "v1", code: "ABC-1" },
      { videoId: 3, contentVersion: "v3", code: "NEW" },
      { videoId: 7, contentVersion: "v7", code: "V-1" },
    ]);
  });
});
