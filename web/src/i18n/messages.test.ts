import { afterEach, describe, expect, it } from "vitest";

import { englishMessages, replaceMessages, t, type MessageSource } from "./messages";

afterEach(() => replaceMessages(englishMessages));

describe("replaceMessages", () => {
  it("updates existing catalog branches without replacing their identity", () => {
    const common = t.common;
    replaceMessages({
      ...englishMessages,
      common: { ...englishMessages.common, close: "Changed" },
    });
    expect(t.common).toBe(common);
    expect(common.close).toBe("Changed");
  });

  it("rejects prototype keys and unknown keys at every depth", () => {
    const payload = JSON.parse(`{
      "__proto__": { "catalogPolluted": true },
      "constructor": { "prototype": { "catalogPolluted": true } },
      "prototype": { "catalogPolluted": true },
      "unknown": "unexpected",
      "common": {
        "__proto__": { "catalogPolluted": true },
        "constructor": { "prototype": { "catalogPolluted": true } },
        "prototype": { "catalogPolluted": true },
        "unknown": "unexpected",
        "close": "Changed"
      }
    }`) as MessageSource;

    try {
      replaceMessages(payload);
      expect(Object.hasOwn(Object.prototype, "catalogPolluted")).toBe(false);
      for (const node of [t, t.common]) {
        expect(Object.getPrototypeOf(node)).toBe(Object.prototype);
        for (const key of ["__proto__", "constructor", "prototype", "unknown"]) {
          expect(Object.hasOwn(node, key)).toBe(false);
        }
      }
      expect(t.common.close).toBe("Changed");
    } finally {
      Reflect.deleteProperty(Object.prototype, "catalogPolluted");
    }
  });
});
