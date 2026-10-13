import type { ComponentProps } from "react";

import { t } from "@/i18n";
import { nameReason } from "@/lib/tagName";
import NameCommand, {
  type NameChoice,
  type NameCommandLayout,
  type NameRules,
} from "./NameCommand";

/** TagChoice はタグの候補の 1 行である。 */
export type TagChoice = NameChoice;

/** TagCommandLayout は候補の一覧の置き方である（NameCommandLayout）。 */
export type TagCommandLayout = NameCommandLayout;

// 文言は表示の時点のカタログから引く（擬似ロケールがカタログを差し替えるため）。
const tagNameRules: NameRules = {
  reason: nameReason,
  get required() {
    return t.tagName.required;
  },
  get controlCharacters() {
    return t.tagName.controlCharacters;
  },
};

/**
 * TagCommand はタグの名前を打って候補から選ぶか作る入力で、NameCommand にタグの名前の
 * 規則を渡したものである（web/registry/rules/components.md「TagCommand」）。
 *
 * - 候補の絞り込みと並び替えは呼び手が行う（`shouldFilter={false}`）。綴りが完全に一致する
 *   名前・シノニムがあれば `exactChoice` に渡す。これが無く `createLabel` があるときだけ
 *   作成の行を出す。
 * - 矢印で行を選ばずに Enter を押すと、打った綴りで決める: 空なら理由を出し、
 *   `exactChoice` があればそれを、無ければ作成の行があれば作る。
 * - 名前は打つたびに確かめ、作れない間（と送信中）は Enter もクリックも受けない。
 *   改行を含む貼り付けと落とし込みは止めて理由を出す。IME の変換中のキーは扱わない。
 */
export default function TagCommand(
  props: Omit<
    ComponentProps<typeof NameCommand>,
    "rules" | "offerCreateWithExact" | "onCommitSpelling"
  >,
) {
  return <NameCommand rules={tagNameRules} {...props} />;
}
