import { TriangleAlert } from "lucide-react";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";

import { RequestFailed } from "../../api/client";
import { errorText, t, type UiText } from "../../i18n";
import { ConfirmDialog } from "../../ui/patterns/confirm-dialog";
import { FormPageDialog } from "../../ui/patterns/form-page";
import { Button } from "../../ui/shadcn/button";
import { Checkbox } from "../../ui/shadcn/checkbox";
import { Field, FieldDescription, FieldError, FieldLabel } from "../../ui/shadcn/field";
import { Input } from "../../ui/shadcn/input";
import { Separator } from "../../ui/shadcn/separator";
import { codeInputId, CodeTable } from "./CodeTable";
import {
  applyItems,
  type CodeEditItem,
  type CodeEditRow,
  deriveRows,
  nextPattern,
  noPattern,
  type PatternState,
} from "./rows";

interface Failure {
  notice: UiText;
  /** The row the error's `items[i]` field names, with the reason for under its input. */
  row?: { videoId: number; reason: string };
}

/** Maps `items[i].…` back to the video of the i-th item sent. */
function failedRow(
  error: unknown,
  sent: readonly CodeEditItem[],
): { videoId: number; reason: string } | undefined {
  if (!(error instanceof RequestFailed) || error.field === undefined) return undefined;
  const match = /^items\[(\d+)\]/.exec(error.field);
  const item = match === null ? undefined : sent[Number(match[1])];
  return item === undefined
    ? undefined
    : { videoId: item.videoId, reason: errorText(error) };
}

/**
 * The code editor over the selected cards: one row per summary row, the pattern controls and the
 * counts in the aside. It sends nothing: `onApply` sends the items and resolves once they are
 * written, and the caller closes the editor then; a rejection keeps every value and shows why.
 * Mount only while open.
 */
export default function CodeEditor({
  rows,
  onApply,
  onClose,
}: {
  rows: readonly CodeEditRow[];
  onApply: (items: CodeEditItem[]) => Promise<void>;
  onClose: () => void;
}) {
  const b = t.library.bulkEdit;
  const c = t.library.codeEdit;
  const [pattern, setPattern] = useState("");
  const [replacement, setReplacement] = useState("");
  const [compiled, setCompiled] = useState<PatternState>({ compiled: noPattern });
  const [upperCase, setUpperCase] = useState(false);
  const [typed, setTyped] = useState<ReadonlyMap<number, string>>(new Map());
  const [pending, setPending] = useState(false);
  const [failure, setFailure] = useState<Failure | null>(null);
  const [confirmOpen, setConfirmOpen] = useState(false);
  const patternInput = useRef<HTMLInputElement>(null);

  const derived = useMemo(
    () => deriveRows({ rows, pattern: compiled.compiled, upperCase, typed }),
    [rows, compiled, upperCase, typed],
  );
  const writing = derived.filter((row) => row.writes).length;
  const duplicates = derived.filter((row) => row.duplicate).length;
  const changed = typed.size > 0 || pattern !== "" || replacement !== "" || upperCase;

  const failedVideoId = failure?.row?.videoId;
  useEffect(() => {
    if (failedVideoId === undefined) return;
    const input = document.getElementById(codeInputId(failedVideoId));
    input?.scrollIntoView?.({ block: "center" });
    input?.focus();
  }, [failure, failedVideoId]);

  function shape(nextPatternText: string, nextReplacement: string) {
    if (pending) return;
    setFailure(null);
    setPattern(nextPatternText);
    setReplacement(nextReplacement);
    setCompiled((last) => nextPattern(nextPatternText, nextReplacement, last.compiled));
  }

  const onType = useCallback((videoId: number, value: string) => {
    setFailure(null);
    setTyped((current) => new Map(current).set(videoId, value));
  }, []);
  const onRestore = useCallback((videoId: number) => {
    setFailure(null);
    setTyped((current) => {
      const next = new Map(current);
      next.delete(videoId);
      return next;
    });
  }, []);

  function requestClose() {
    if (pending) return;
    if (changed) setConfirmOpen(true);
    else onClose();
  }

  function apply() {
    if (pending || writing === 0) return;
    const items = applyItems(derived);
    setPending(true);
    setFailure(null);
    onApply(items).catch((error: unknown) => {
      setFailure({ notice: c.failed(errorText(error)), row: failedRow(error, items) });
      setPending(false);
    });
  }

  const patternError = compiled.error;
  return (
    <>
      <FormPageDialog
        open
        onClose={requestClose}
        initialFocus={patternInput}
        title={c.title}
        detail={b.detail(rows.length)}
        actions={
          <>
            <Button
              variant="ghost"
              size="sm"
              aria-disabled={pending || undefined}
              onClick={requestClose}
            >
              {t.player.detailsEditor.cancel}
            </Button>
            <Button
              size="sm"
              aria-disabled={writing === 0 || pending || undefined}
              aria-busy={pending || undefined}
              onClick={apply}
            >
              {pending ? b.applying : b.apply(writing)}
            </Button>
          </>
        }
        notice={
          failure !== null && (
            <p role="alert" className="text-sm text-destructive">
              {failure.notice}
            </p>
          )
        }
        asideLabel={c.suggestions}
        aside={
          <>
            <h2 className="font-medium">{c.suggestions}</h2>
            <p className="text-xs text-muted-foreground">{c.hint}</p>
            <div className="grid gap-3 md:max-lg:grid-cols-2">
              <Field data-invalid={patternError !== undefined || undefined}>
                <FieldLabel htmlFor="code-edit-pattern">{c.pattern}</FieldLabel>
                <Input
                  ref={patternInput}
                  id="code-edit-pattern"
                  className="h-8 font-mono"
                  aria-invalid={patternError !== undefined || undefined}
                  aria-describedby={
                    patternError === undefined
                      ? "code-edit-pattern-hint"
                      : "code-edit-pattern-hint code-edit-pattern-error"
                  }
                  aria-disabled={pending || undefined}
                  value={pattern}
                  onChange={(event) => shape(event.target.value, replacement)}
                />
                <FieldDescription id="code-edit-pattern-hint">
                  {c.patternHint}
                </FieldDescription>
                {patternError !== undefined && (
                  <FieldError id="code-edit-pattern-error">
                    {c.invalidPattern(patternError)}
                  </FieldError>
                )}
              </Field>
              <Field>
                <FieldLabel htmlFor="code-edit-replacement">{c.replacement}</FieldLabel>
                <Input
                  id="code-edit-replacement"
                  className="h-8"
                  aria-describedby="code-edit-replacement-hint"
                  aria-disabled={pending || undefined}
                  value={replacement}
                  onChange={(event) => shape(pattern, event.target.value)}
                />
                <FieldDescription id="code-edit-replacement-hint">
                  {c.replacementHint}
                </FieldDescription>
              </Field>
            </div>
            <Field orientation="horizontal">
              <Checkbox
                id="code-edit-upper-case"
                checked={upperCase}
                disabled={pending}
                onCheckedChange={(next) => {
                  setFailure(null);
                  setUpperCase(next === true);
                }}
              />
              <FieldLabel htmlFor="code-edit-upper-case" className="font-normal">
                {c.upperCase}
              </FieldLabel>
            </Field>
            <Separator />
            <p className="text-sm tabular-nums">{c.willWrite(writing, rows.length)}</p>
            {duplicates >= 2 && (
              <p className="flex items-center gap-1 text-sm text-warning tabular-nums">
                <TriangleAlert aria-hidden="true" className="size-4 shrink-0" />
                {c.duplicates(duplicates)}
              </p>
            )}
          </>
        }
        panels={[
          {
            key: "codes",
            legend: c.codes,
            content: (
              <CodeTable
                rows={derived}
                typed={typed}
                disabled={pending}
                failed={failure?.row}
                onType={onType}
                onRestore={onRestore}
              />
            ),
          },
        ]}
      />
      <ConfirmDialog
        open={confirmOpen}
        onOpenChange={setConfirmOpen}
        title={b.discardTitle}
        description={c.discardText}
        actionLabel={t.player.detailsEditor.discard}
        cancelLabel={t.player.detailsEditor.keepEditing}
        onConfirm={onClose}
      />
    </>
  );
}
