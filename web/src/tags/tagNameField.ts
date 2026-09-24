import { useState, type ClipboardEvent, type FormEvent } from "react";

import { nameReason, newlinePattern } from "../ui/Combobox";

/**
 * useTagNameField は、タグの名前を打つ入力の検証と、改行を含む貼り付け・
 * 落とし込みの遮断を持つ（ui-design.md「Combobox」名前の検証・改行の扱い）。
 * 管理画面の作成・改名の入力は `ui/Combobox` の候補の一覧を持たないが、
 * 検証と理由の出し方は同じにする（Issue 271「作成と改名」）。
 *
 * 理由は、貼り付け・落とし込みで直接 `setReason` した分も含め、次に値が
 * 実際に変わるまで残る（`setValue` を呼んだときだけ検証をやり直す）。
 */
export function useTagNameField(initial = "") {
  const [value, setValueState] = useState(initial);
  const [reason, setReason] = useState<string | null>(nameReason(initial));

  function setValue(next: string): void {
    setValueState(next);
    setReason(nameReason(next));
  }

  function onPaste(event: ClipboardEvent<HTMLInputElement>): void {
    const text = event.clipboardData.getData("text");
    if (newlinePattern.test(text)) {
      event.preventDefault();
      setReason("改行やタブは使えません");
    }
  }

  function onBeforeInput(event: FormEvent<HTMLInputElement>): void {
    const native = event.nativeEvent as InputEvent;
    if (native.inputType !== "insertFromDrop") return;
    const text = native.data ?? "";
    if (newlinePattern.test(text)) {
      event.preventDefault();
      setReason("改行やタブは使えません");
    }
  }

  /** trySpelling は Enter・「作成」「確定」で呼ぶ。前後の空白を除いた綴りを返す。 */
  function trySpelling(): string | null {
    if (reason !== null) return null;
    const trimmed = value.trim();
    if (trimmed === "") {
      setReason("名前を入力してください");
      return null;
    }
    return trimmed;
  }

  return { value, setValue, reason, setReason, onPaste, onBeforeInput, trySpelling };
}
