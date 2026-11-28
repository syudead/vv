// @vitest-environment node
import { describe, expect, it } from "vitest";

import { t } from "../i18n";
import {
  expectNameRule,
  readDomainFixture,
  type NameRuleFile,
} from "../testing/domainFixture";
import { creditNameMaxLength, creditNameReason } from "./creditName";

const file = readDomainFixture<NameRuleFile>("credit_name.json");

describe("creditNameReason", () => {
  it("共有の組で Go の NormalizeCreditName と同じ上限を持つ", () => {
    expect(creditNameMaxLength).toBe(file.maxLength);
  });

  it("共有の組で Go の NormalizeCreditName と同じ名前を受け付け、同じ理由で拒む", () => {
    expectNameRule(file, creditNameReason, {
      controlCharacters: t.creditName.controlCharacters,
      tooLong: () => t.creditName.tooLong(creditNameMaxLength),
    });
  });
});
