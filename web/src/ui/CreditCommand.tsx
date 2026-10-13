import type { ComponentProps } from "react";

import { t } from "@/i18n";
import { creditNameReason } from "@/lib/creditName";
import NameCommand, { type NameChoice, type NameRules } from "./NameCommand";

/** CreditChoice は人物か団体の候補の 1 行である。 */
export type CreditChoice = NameChoice;

// 文言は表示の時点のカタログから引く（擬似ロケールがカタログを差し替えるため）。
const creditNameRules: NameRules = {
  reason: creditNameReason,
  get required() {
    return t.creditName.required;
  },
  get controlCharacters() {
    return t.creditName.controlCharacters;
  },
};

/**
 * CreditCommand は人物か団体の名前を打って候補から選ぶか作る入力で、NameCommand
 * （TagCommand と同じ基底）に人物・団体の名前の規則（1〜200 符号位置、制御文字なし）を
 * 渡したものである（web/registry/rules/components.md「CreditCommand」）。
 *
 * TagCommand と違い、綴りが一致する人物がいても作成の行を出す（同名の別人を作れるように。
 * specs/047-video-work-info/ui-design.md「Why this shape」）。矢印を押さずに Enter を
 * 押したときの決め方は呼び手が `onCommitSpelling` で持つ（候補の外の完全一致を探すため）。
 */
export default function CreditCommand(
  props: Omit<ComponentProps<typeof NameCommand>, "rules" | "offerCreateWithExact">,
) {
  return <NameCommand rules={creditNameRules} offerCreateWithExact {...props} />;
}
