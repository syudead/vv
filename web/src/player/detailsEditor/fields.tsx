import type { KeyboardEvent } from "react";

import type { UiText } from "../../i18n";
import { isComposingKeyEvent } from "../../lib/ime";
import { FieldError } from "../../ui/shadcn/field";

// 編集画面の入力が分け合う小さな部品と値。

/** FieldProblem は、保存の誤りの `field` が指した入力とその理由である。 */
export interface FieldProblem {
  /** 入力の要素の id の末尾（`details-${control}`）。 */
  control: string;
  text: UiText;
}

/** controlId は入力の要素の id である。 */
export function controlId(control: string): string {
  return `details-${control}`;
}

/** errorId は入力の理由の要素の id である。 */
export function errorId(control: string): string {
  return `details-${control}-error`;
}

/** invalidProps は、理由がその入力を指すときに入力へ付ける属性である。 */
export function invalidProps(
  problem: FieldProblem | null,
  control: string,
): { "aria-invalid"?: true; "aria-describedby"?: string } {
  if (problem?.control !== control) return {};
  return { "aria-invalid": true, "aria-describedby": errorId(control) };
}

/** Problem は、理由がその入力を指すときだけ、その下に理由を出す。 */
export function Problem({
  problem,
  control,
}: {
  problem: FieldProblem | null;
  control: string;
}) {
  if (problem?.control !== control) return null;
  return <FieldError id={errorId(control)}>{problem.text}</FieldError>;
}

/**
 * saveOnEnter は、1 行の入力で Enter を押したら保存する（タイトルのその場編集と同じ。
 * ui-design.md「Fields」の Keys）。IME の変換を決める Enter では保存しない。
 */
export function saveOnEnter(save: () => void) {
  return (event: KeyboardEvent<HTMLInputElement>) => {
    if (event.key !== "Enter" || isComposingKeyEvent(event)) return;
    event.preventDefault();
    save();
  };
}
