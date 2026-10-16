import { describe, expect, it } from "vitest";

import {
  activityConditions,
  DEFAULT_ACTIVITY_CRITERIA,
  hasActivityConditions,
  parseActivityCriteria,
  periodSince,
  serializeActivityCriteria,
} from "./activityCriteria";

function parse(search: string) {
  return parseActivityCriteria(new URLSearchParams(search));
}

describe("parseActivityCriteria", () => {
  it("reads every filter from the URL", () => {
    expect(
      parse(
        "status=attention&feature=tags&feature=import&decider=ai&period=week&q=%20cat%20",
      ),
    ).toEqual({
      status: "attention",
      features: ["import", "tags"],
      decider: "ai",
      period: "week",
      q: "cat",
    });
  });

  it("treats unknown values as the default", () => {
    expect(parse("status=nope&feature=x&decider=bot&period=year")).toEqual(
      DEFAULT_ACTIVITY_CRITERIA,
    );
  });
});

describe("serializeActivityCriteria", () => {
  it("writes no default and keeps a fixed order", () => {
    expect(serializeActivityCriteria(DEFAULT_ACTIVITY_CRITERIA).toString()).toBe("");
    expect(
      serializeActivityCriteria({
        q: "a",
        period: "today",
        decider: "owner",
        features: ["versions", "scan"],
        status: "done",
      }).toString(),
    ).toBe("status=done&feature=scan&feature=versions&decider=owner&period=today&q=a");
  });

  it("round-trips through the URL", () => {
    const search = "status=running&feature=auto_tagging&period=month";
    expect(serializeActivityCriteria(parse(search)).toString()).toBe(search);
    expect(hasActivityConditions(parse(search))).toBe(true);
    expect(hasActivityConditions(parse(""))).toBe(false);
  });
});

describe("periodSince", () => {
  const now = new Date(2026, 9, 16, 14, 30);

  it("starts today at local midnight and the others that many days ago", () => {
    expect(periodSince("any", now)).toBeUndefined();
    expect(periodSince("today", now)).toEqual(new Date(2026, 9, 16));
    expect(periodSince("week", now)?.getTime()).toBe(
      now.getTime() - 7 * 24 * 60 * 60 * 1000,
    );
    expect(periodSince("month", now)?.getTime()).toBe(
      now.getTime() - 30 * 24 * 60 * 60 * 1000,
    );
  });

  it("turns the period into since for the request", () => {
    expect(
      activityConditions({ ...DEFAULT_ACTIVITY_CRITERIA, period: "today" }, now),
    ).toEqual({ since: new Date(2026, 9, 16).toISOString() });
    expect(activityConditions(DEFAULT_ACTIVITY_CRITERIA, now)).toEqual({});
  });
});
