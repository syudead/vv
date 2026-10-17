import { useEffect, useRef } from "react";

import type { UiText } from "../i18n";
import { isComposingKeyEvent } from "../lib/ime";
import type { NameRules } from "../ui/NameCommand";
import { Input } from "../ui/shadcn/input";
import { useTagNameField, type TagFieldError } from "./tagNameField";

/**
 * RowNameInput は管理画面の行の、その場の改名の入力である。改名を始めると描かれ、今の名前を選んだ状態でフォーカスを持つ。Enter で
 * 送り、Esc で閉じる（送信中は閉じない）。理由は入力の下に出す: 名前の規則と
 * `taken` の失敗は `text-xs`、それ以外の失敗は `role="alert"` の `text-sm`。
 *
 * タグの行（`TagRow`）と人物・団体の行（`web/src/credits/CreditRow`）が使う。
 */
export default function RowNameInput({
  idPrefix,
  rowId,
  name,
  rules,
  label,
  pending,
  error,
  onSubmit,
  onCancel,
  onDraftChange,
}: {
  /** 理由の要素の id の頭（`tag-rename`）。 */
  idPrefix: string;
  rowId: number;
  /** 今の名前。改名を始めたときの値。 */
  name: string;
  /** 名前の規則。省くとタグの名前の規則。 */
  rules?: NameRules;
  label: UiText;
  pending: boolean;
  error: TagFieldError | null;
  onSubmit: (name: string) => void;
  onCancel: () => void;
  /** 値が変わるたびに呼ぶ。呼び出し元はこれで直前の失敗の表示を消す。 */
  onDraftChange?: () => void;
}) {
  const field = useTagNameField(name, onDraftChange, rules);
  const inputRef = useRef<HTMLInputElement | null>(null);

  // 改名を始めたときに今の名前を選んだ状態にする。
  useEffect(() => {
    inputRef.current?.focus();
    inputRef.current?.select();
  }, []);

  function submit() {
    if (pending) return;
    const spelling = field.trySpelling();
    if (spelling !== null) onSubmit(spelling);
  }

  function cancel() {
    // 送信中は、その応答が届くまで閉じない（Esc の場合。行に「キャンセル」のボタンは
    // 無い）。閉じたあとに届いた応答が、もう無いこの行や別の行の状態を書き換えることを防ぐ。
    if (pending) return;
    onCancel();
  }

  const reasonId = `${idPrefix}-reason-${String(rowId)}`;
  const errorId = `${idPrefix}-error-${String(rowId)}`;

  return (
    <>
      <Input
        ref={inputRef}
        value={field.value}
        onChange={(event) => field.setValue(event.target.value)}
        onPaste={field.onPaste}
        onBeforeInput={field.onBeforeInput}
        onKeyDown={(event) => {
          if (isComposingKeyEvent(event)) return;
          if (event.key === "Enter") {
            event.preventDefault();
            submit();
          } else if (event.key === "Escape") {
            event.preventDefault();
            cancel();
          }
        }}
        aria-label={label}
        aria-describedby={
          field.reason !== null ? reasonId : error !== null ? errorId : undefined
        }
        aria-busy={pending || undefined}
        aria-invalid={field.reason !== null || error?.kind === "taken" || undefined}
        className="h-8"
      />
      {field.reason !== null && (
        <p id={reasonId} className="mt-1 text-xs text-destructive">
          {field.reason}
        </p>
      )}
      {field.reason === null && error !== null && error.kind === "taken" && (
        <p id={errorId} className="mt-1 text-xs text-destructive">
          {error.message}
        </p>
      )}
      {field.reason === null && error !== null && error.kind === "other" && (
        <p id={errorId} role="alert" className="mt-1 text-sm text-destructive">
          {error.message}
        </p>
      )}
    </>
  );
}
