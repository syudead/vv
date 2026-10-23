import { describe, expect, it } from "vitest";

import {
  formatDate,
  formatDateTime,
  formatMonth,
  formatMonthDay,
  formatNumber,
  formatPreciseTime,
  formatRelative,
  formatTime,
  formatWeekday,
  formatWeekdayDate,
} from "./format";
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
  // Use local noon to keep the intended date in browser-timezone formatting.
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

  it("formats a time of day in English", () => {
    expect(formatTime(new Date(2026, 8, 27, 15, 4))).toMatch(/^3:04\s?PM$/);
  });

  it("formats a precise time with seconds, and milliseconds only when the timestamp has them", () => {
    const seconds = new Date(2026, 8, 27, 15, 4, 5).toISOString();
    expect(formatPreciseTime(seconds)).toMatch(/^3:04:05\s?PM$/);
    const precise = new Date(2026, 8, 27, 15, 4, 5, 120).toISOString();
    expect(formatPreciseTime(precise)).toMatch(/^3:04:05\.120\s?PM$/);
    // A fraction below one millisecond reads as whole seconds.
    expect(formatPreciseTime(precise.replace(".120Z", ".000456789Z"))).toMatch(
      /^3:04:05\s?PM$/,
    );
  });

  it("formats the weekday in English", () => {
    expect(formatWeekday(noon)).toBe("Sunday");
  });

  it("formats the month and day, with the year only outside the current year", () => {
    expect(formatMonthDay(noon, new Date(2026, 9, 10))).toBe("Sep 27");
    expect(formatMonthDay(noon, new Date(2027, 0, 2))).toBe("Sep 27, 2026");
  });

  it("formats the short weekday with the date, and the month, with the year only outside the current year", () => {
    expect(formatWeekdayDate(noon, new Date(2026, 9, 10))).toBe("Sun, Sep 27");
    expect(formatWeekdayDate(noon, new Date(2027, 0, 2))).toBe("Sun, Sep 27, 2026");
    expect(formatMonth(noon, new Date(2026, 9, 10))).toBe("September");
    expect(formatMonth(noon, new Date(2027, 0, 2))).toBe("September 2026");
  });

  it("returns an empty string for unreadable dates", () => {
    expect(formatWeekday("nope")).toBe("");
    expect(formatMonthDay("nope")).toBe("");
    expect(formatWeekdayDate("nope")).toBe("");
    expect(formatMonth("nope")).toBe("");
    expect(formatTime("nope")).toBe("");
    expect(formatDate("nope")).toBe("");
    expect(formatDateTime("nope")).toBe("");
    expect(formatPreciseTime("nope")).toBe("");
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
