import type { ComponentProps } from "react";

import { t } from "@/i18n";
import { creditNameReason } from "@/lib/creditName";
import NameCommand, { type NameChoice, type NameRules } from "./NameCommand";

export type CreditChoice = NameChoice;

// Read catalog copy at render time so pseudo-locale replacements propagate.
export const creditNameRules: NameRules = {
  reason: creditNameReason,
  get required() {
    return t.creditName.required;
  },
  get controlCharacters() {
    return t.creditName.controlCharacters;
  },
};

/**
 * Allow creation even with an exact spelling match because distinct people may share names. The
 * caller resolves unselected Enter through onCommitSpelling, including exact matches outside
 * loaded candidates.
 */
export default function CreditCommand(
  props: Omit<ComponentProps<typeof NameCommand>, "rules" | "offerCreateWithExact">,
) {
  return <NameCommand rules={creditNameRules} offerCreateWithExact {...props} />;
}
