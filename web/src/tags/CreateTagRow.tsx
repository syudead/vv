import { useEffect, useRef } from "react";

import Button from "../ui/Button";
import { isComposingKeyEvent } from "../ui/Combobox";
import { useTagNameField, type TagFieldError } from "./tagNameField";

/**
 * CreateTagRow は「新しいタグ」で一覧の先頭に差し込む作成の行である
 * （ui-design.md「Create and rename」）。候補の一覧は持たない、名前1つだけの
 * 入力で、検証と理由の出し方は `ui/Combobox` と同じにする。
 */
export default function CreateTagRow({
  pending,
  error,
  onCancel,
  onSubmit,
  onDraftChange,
}: {
  pending: boolean;
  error: TagFieldError | null;
  onCancel: () => void;
  onSubmit: (name: string) => void;
  /** 値が変わるたびに呼ぶ。呼び出し元はこれで直前の失敗の表示を消す。 */
  onDraftChange?: () => void;
}) {
  const field = useTagNameField("", onDraftChange);
  const inputRef = useRef<HTMLInputElement | null>(null);

  useEffect(() => {
    inputRef.current?.focus();
  }, []);

  function submit() {
    if (pending) return;
    const spelling = field.trySpelling();
    if (spelling !== null) onSubmit(spelling);
  }

  function cancel() {
    // 送信中は、その応答が届くまで閉じない（Esc・「キャンセル」の両方。
    // 閉じたあとに届いた応答が、もう無いこの行や別の行の状態を書き換える
    // ことを防ぐ。ModalFrame の pending の扱いと同じ）。
    if (pending) return;
    onCancel();
  }

  return (
    <div className="flex flex-wrap items-center gap-2 py-2">
      <input
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
        placeholder="タグの名前"
        aria-label="新しいタグの名前"
        aria-describedby={
          field.reason !== null
            ? "tag-create-reason"
            : error !== null
              ? "tag-create-error"
              : undefined
        }
        aria-busy={pending || undefined}
        className="h-8 min-w-0 flex-1 rounded-sm border border-border bg-field px-2 text-sm text-fg focus:border-accent focus:outline-none"
      />
      <div className="flex shrink-0 items-center gap-2">
        <Button variant="primary" size="sm" onClick={submit} disabled={pending}>
          作成
        </Button>
        <Button variant="ghost" size="sm" onClick={cancel} disabled={pending}>
          キャンセル
        </Button>
      </div>
      {field.reason !== null && (
        <p id="tag-create-reason" className="w-full text-xs text-danger">
          {field.reason}
        </p>
      )}
      {field.reason === null && error !== null && error.kind === "taken" && (
        <p id="tag-create-error" className="w-full text-xs text-danger">
          {error.message}
        </p>
      )}
      {field.reason === null && error !== null && error.kind === "other" && (
        <p id="tag-create-error" role="alert" className="w-full text-sm text-danger">
          {error.message}
        </p>
      )}
    </div>
  );
}
