import type { ComponentProps } from "react";

import { t } from "@/i18n";
import { nameReason } from "@/lib/tagName";
import NameCommand, {
  type NameChoice,
  type NameCommandLayout,
  type NameRules,
} from "./NameCommand";

export type TagChoice = NameChoice;

export type TagCommandLayout = NameCommandLayout;

// Read catalog copy at render time so pseudo-locale replacements propagate.
export const tagNameRules: NameRules = {
  reason: nameReason,
  get required() {
    return t.tagName.required;
  },
  get controlCharacters() {
    return t.tagName.controlCharacters;
  },
};

/**
 * Use NameCommand's filtering and Enter contracts with shared tag-name validation; see
 * web/registry/rules/components.md, TagCommand.
 */
export default function TagCommand(
  props: Omit<
    ComponentProps<typeof NameCommand>,
    "rules" | "offerCreateWithExact" | "onCommitSpelling"
  >,
) {
  return <NameCommand rules={tagNameRules} {...props} />;
}
