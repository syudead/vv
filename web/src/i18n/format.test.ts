import { describe, expect, it } from "vitest";

import { formatDate, formatDateTime, formatNumber, formatRelative } from "./format";
import { t } from "./messages";

describe("counts", () => {
  it("chooses the singular for one and the plural otherwise", () => {
    expect(t.count.videos(1)).toBe("1 video");
    expect(t.count.videos(0)).toBe("0 videos");
    expect(t.count.videos(2)).toBe("2 videos");
    expect(t.count.videos(12_345)).toBe("12,345 videos");
  });

  it("formats numbers with English digit grouping", () => {
    expect(formatNumber(1_234_567)).toBe("1,234,567");
  });
});

describe("dates", () => {
  // 日時はブラウザのタイムゾーンで表すので、その日の正午を渡して日付が変わらないようにする。
  const noon = new Date(2026, 8, 27, 12, 0);

  it("formats a date in English", () => {
    expect(formatDate(noon)).toBe("Sep 27, 2026");
    expect(formatDate(noon.toISOString())).toBe("Sep 27, 2026");
  });

  it("formats a date and time in English", () => {
    expect(formatDateTime(new Date(2026, 8, 27, 15, 4))).toMatch(
      /^Sep 27, 2026, 3:04\s?PM$/,
    );
  });

  it("returns an empty string for unreadable dates", () => {
    expect(formatDate("nope")).toBe("");
    expect(formatDateTime("nope")).toBe("");
  });
});

describe("formatRelative", () => {
  const now = new Date("2026-09-20T12:00:00Z");

  it("keeps the same steps as before, in English", () => {
    expect(formatRelative("2026-09-20T11:59:50Z", now)).toBe("just now");
    expect(formatRelative("2026-09-20T11:59:00Z", now)).toBe("1 minute ago");
    expect(formatRelative("2026-09-20T11:30:00Z", now)).toBe("30 minutes ago");
    expect(formatRelative("2026-09-20T06:00:00Z", now)).toBe("6 hours ago");
    expect(formatRelative("2026-09-19T12:00:00Z", now)).toBe("1 day ago");
    expect(formatRelative("2026-09-14T12:00:00Z", now)).toBe("6 days ago");
    expect(formatRelative("2026-08-30T12:00:00Z", now)).toBe("3 weeks ago");
    expect(formatRelative("2026-03-20T12:00:00Z", now)).toBe("6 months ago");
    expect(formatRelative("2020-09-20T12:00:00Z", now)).toBe("6 years ago");
  });

  it("returns an empty string for unreadable dates", () => {
    expect(formatRelative("nope", now)).toBe("");
  });
});
