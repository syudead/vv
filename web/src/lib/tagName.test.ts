// @vitest-environment node
import { describe, expect, it } from "vitest";

import { t } from "../i18n";
import {
  expectNameRule,
  readDomainFixture,
  type NameRuleFile,
} from "../testing/domainFixture";
import { nameReason, tagNameMaxLength } from "./tagName";

const file = readDomainFixture<NameRuleFile>("tag_name.json");

describe("nameReason", () => {
  it("共有の組で Go の NormalizeTagName と同じ上限を持つ", () => {
    expect(tagNameMaxLength).toBe(file.maxLength);
  });

  it("共有の組で Go の NormalizeTagName と同じ名前を受け付け、同じ理由で拒む", () => {
    expectNameRule(file, nameReason, {
      controlCharacters: t.tagName.controlCharacters,
      tooLong: (length) => t.tagName.tooLong(tagNameMaxLength, length),
    });
  });
});
