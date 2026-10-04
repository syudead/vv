import { useEffect, useRef } from "react";

import { t } from "../i18n";
import { isComposingKeyEvent } from "../ui/Combobox";
import { Button } from "../ui/shadcn/button";
import { Field, FieldError } from "../ui/shadcn/field";
import { Input } from "../ui/shadcn/input";
import { Spinner } from "../ui/shadcn/spinner";
import { TableCell, TableRow } from "../ui/shadcn/table";
import { useTagNameField, type TagFieldError } from "./tagNameField";

/**
 * CreateTagRow は「新しいタグ」で表の先頭に差し込む作成の行である
 * （ui-design.md「Create and rename」）。候補の一覧は持たない、名前1つだけの
 * 入力で、検証と理由の出し方は `ui/Combobox` と同じにする。表の行（`TableRow`）で、
 * 1 つの欄が列をすべてまたぐ。
 */
export default function CreateTagRow({
  columnCount,
  pending,
  error,
  onCancel,
  onSubmit,
  onDraftChange,
}: {
  /** 表の列の数。作成の欄はすべての列をまたぐ。 */
  columnCount: number;
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
    <TableRow className="hover:bg-transparent">
      <TableCell colSpan={columnCount} className="whitespace-normal">
        <Field data-invalid={field.reason !== null || error !== null || undefined}>
          <div className="flex flex-wrap items-center gap-2">
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
              placeholder={t.tags.create.placeholder}
              aria-label={t.tags.create.label}
              aria-describedby={
                field.reason !== null
                  ? "tag-create-reason"
                  : error !== null
                    ? "tag-create-error"
                    : undefined
              }
              aria-busy={pending || undefined}
              aria-invalid={field.reason !== null || error?.kind === "taken" || undefined}
              className="h-8 basis-full sm:flex-1 sm:basis-auto"
            />
            <div className="flex w-full items-center justify-end gap-2 sm:w-auto">
              <Button size="sm" onClick={submit} disabled={pending}>
                {pending && <Spinner aria-hidden="true" />}
                {pending ? t.tags.create.submitting : t.tags.create.submit}
              </Button>
              <Button variant="outline" size="sm" onClick={cancel} disabled={pending}>
                {t.common.cancel}
              </Button>
            </div>
          </div>
          {field.reason !== null && (
            <FieldError id="tag-create-reason" role={undefined}>
              {field.reason}
            </FieldError>
          )}
          {field.reason === null && error !== null && error.kind === "taken" && (
            <FieldError id="tag-create-error" role={undefined}>
              {error.message}
            </FieldError>
          )}
          {field.reason === null && error !== null && error.kind === "other" && (
            <FieldError id="tag-create-error" className="text-sm">
              {error.message}
            </FieldError>
          )}
        </Field>
      </TableCell>
    </TableRow>
  );
}
