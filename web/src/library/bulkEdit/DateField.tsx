import { useId } from "react";

import type { VideoEditSummary } from "../../api/videoEdits";
import type { WorkDateKind } from "../../api/workInfo";
import { t } from "../../i18n";
import { Field, FieldDescription, FieldLabel } from "../../ui/shadcn/field";
import { Input } from "../../ui/shadcn/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "../../ui/shadcn/select";
import type { DateDraft } from "./draft";

type Mode = "keep" | DateDraft["mode"];

/** One date kind: what the videos have now, and Keep, Set to a date or Clear. */
export default function DateField({
  kind,
  total,
  current,
  draft,
  disabled,
  onChange,
}: {
  kind: WorkDateKind;
  total: number;
  current: VideoEditSummary["dates"][number] | undefined;
  draft: DateDraft | undefined;
  disabled: boolean;
  onChange: (draft: DateDraft | undefined) => void;
}) {
  const b = t.library.bulkEdit;
  const inputId = useId();
  const mode: Mode = draft?.mode ?? "keep";
  const count = current?.count ?? 0;
  const now =
    count === 0
      ? b.dateNone
      : current?.date !== undefined
        ? b.dateShared(current.date, count, total)
        : b.dateMixed(count, total);
  return (
    <Field>
      <FieldLabel htmlFor={inputId}>{b.dateLabel(kind)}</FieldLabel>
      <FieldDescription>{now}</FieldDescription>
      <div className="flex min-w-0 items-center gap-2">
        <Select
          value={mode}
          disabled={disabled}
          onValueChange={(next) =>
            onChange(
              next === "set"
                ? { mode: "set", value: current?.date ?? "" }
                : next === "clear"
                  ? { mode: "clear" }
                  : undefined,
            )
          }
        >
          <SelectTrigger size="sm" aria-label={b.dateMode(b.dateLabel(kind))}>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="keep">{b.keep}</SelectItem>
            <SelectItem value="set">{b.setDate}</SelectItem>
            <SelectItem value="clear" disabled={count === 0}>
              {b.clear}
            </SelectItem>
          </SelectContent>
        </Select>
        {draft?.mode === "set" && (
          <Input
            id={inputId}
            value={draft.value}
            disabled={disabled}
            placeholder={t.player.detailsEditor.datePlaceholder}
            inputMode="numeric"
            className="h-8 min-w-0 flex-1"
            onChange={(event) => onChange({ mode: "set", value: event.target.value })}
          />
        )}
      </div>
    </Field>
  );
}
