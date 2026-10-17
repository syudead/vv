import { describe, expect, it } from "vitest";

import type { HomeOrganizeRun } from "../api/home";
import { formatDateTime } from "../i18n";
import { lastRunLine, organizerBusy } from "./homeSectionRules";

// 設定の「Home」の節の規則（specs/057-home/ui-design.md「Words」の Settings の行）。

const finishedAt = Date.UTC(2026, 9, 17, 3, 4);
const when = formatDateTime(new Date(finishedAt));

function run(change: Partial<HomeOrganizeRun>): HomeOrganizeRun {
  return { run: "rebuild", outcome: "done", finishedAt, added: 0, removed: 0, ...change };
}

describe("lastRunLine", () => {
  it("足した数と消した数を出す", () => {
    expect(lastRunLine(run({ added: 3, removed: 1 }))).toBe(
      `Last organized ${when}: 3 added, 1 removed.`,
    );
  });

  it("何も変えなかった実行と、失敗と、まだ無い実行を言い分ける", () => {
    expect(lastRunLine(run({ outcome: "unchanged" }))).toBe(
      `Last organized ${when}: nothing changed.`,
    );
    expect(lastRunLine(run({}))).toBe(`Last organized ${when}: nothing changed.`);
    expect(lastRunLine(run({ outcome: "failed" }))).toBe(
      `Last organizing failed ${when}.`,
    );
    expect(lastRunLine(undefined)).toBe("Not organized yet.");
  });
});

describe("organizerBusy", () => {
  it("待ちか実行中なら押せない", () => {
    expect(organizerBusy({ running: false })).toBe(false);
    expect(organizerBusy({ running: true })).toBe(true);
    expect(organizerBusy({ running: false, pending: "extend" })).toBe(true);
  });
});
