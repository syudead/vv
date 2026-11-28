import { readFileSync } from "node:fs";
import { join } from "node:path";

import { expect } from "vitest";

import type { UiText } from "../i18n";
import { trimWhiteSpace, trimmedCodePoints } from "../lib/textRules";

/** Read a fixture of internal/domain/testdata, which Go's internal/domain/foldcheck tests also verify. */
export function readDomainFixture<T>(name: string): T {
  const path = join(__dirname, "..", "..", "..", "internal", "domain", "testdata", name);
  return JSON.parse(readFileSync(path, "utf8")) as T;
}

/** A name-rule case: want when the name is accepted, problem otherwise. */
export interface NameRuleCase {
  note: string;
  input: string;
  want?: string;
  problem?: "empty" | "controlCharacters" | "tooLong";
}

export interface NameRuleFile {
  maxLength: number;
  cases: NameRuleCase[];
}

export interface NameRuleMessages {
  controlCharacters: UiText;
  tooLong: (length: number) => UiText;
}

/**
 * Check a typing-time reason and the spelling the SPA sends against the server's outcome; an empty name has no
 * reason while typing, and callers refuse it when its trimmed spelling is empty.
 */
export function expectNameRule(
  file: NameRuleFile,
  reason: (raw: string) => UiText | null,
  messages: NameRuleMessages,
): void {
  expect(file.cases.length).toBeGreaterThan(0);
  for (const c of file.cases) {
    const want =
      c.problem === "controlCharacters"
        ? messages.controlCharacters
        : c.problem === "tooLong"
          ? messages.tooLong(trimmedCodePoints(c.input))
          : null;
    expect(reason(c.input), c.note).toEqual(want);
    if (c.problem === "empty") expect(trimWhiteSpace(c.input), c.note).toBe("");
    if (c.want !== undefined) expect(trimWhiteSpace(c.input), c.note).toBe(c.want);
  }
}
