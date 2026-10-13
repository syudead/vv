import { ArrowRight, Search } from "lucide-react";
import { type ComponentType, type ReactNode, useCallback, useId, useState } from "react";

import type { UiText } from "../i18n";
import type { NameChoice, NameCommandLayout } from "../ui/NameCommand";
import { Field, FieldError, FieldLabel } from "../ui/shadcn/field";

/** MergeCommandProps は統合先の欄が名前の入力（TagCommand・CreditCommand）へ渡す props である。 */
interface MergeCommandProps {
  layout: NameCommandLayout;
  label: UiText;
  labelledBy: string;
  placeholder: null;
  inputRef: (node: HTMLInputElement | null) => void;
  icon: ReactNode;
  chosenId: string | undefined;
  emptyText: ReactNode;
  value: string;
  onValueChange: (value: string) => void;
  choices: readonly NameChoice[];
  exactChoice: NameChoice | null;
  onSelect: (choice: NameChoice) => void;
  frameClassName: string;
  busy: boolean;
  describedBy: string | undefined;
}

/**
 * MergeTargetField は統合の窓の統合先の欄である。見える名札、入力の下に高さを固定して
 * 並べる候補（`inline`）、候補を引けなかったときの理由を持つ（specs/036-tag-admin-scale/
 * ui-design.md「Merge dialog」「Target candidates」）。名前の規則は `Command`（タグの窓は
 * `TagCommand`、人物・団体の窓は `CreditCommand`）が持つ。Esc は窓が閉じる。
 */
export function MergeTargetField({
  Command,
  label,
  value,
  onValueChange,
  choices,
  exactChoice,
  onSelect,
  chosenId,
  searching,
  searchError,
  emptyText,
}: {
  Command: ComponentType<MergeCommandProps>;
  label: UiText;
  value: string;
  onValueChange: (value: string) => void;
  choices: readonly NameChoice[];
  exactChoice: NameChoice | null;
  onSelect: (choice: NameChoice) => void;
  chosenId: string | undefined;
  searching: boolean;
  /** 候補を引けなかった理由の文（「Couldn't search tags: …」）。引けていれば null。 */
  searchError: UiText | null;
  /** 候補が無いときの文。 */
  emptyText: UiText;
}) {
  const searchErrorId = useId();
  const labelId = useId();
  // 見える名札が指す入力の id。cmdk が入力に自分の id を付けるので、描いた入力から読む。
  const [inputId, setInputId] = useState<string | undefined>(undefined);
  const inputRef = useCallback((node: HTMLInputElement | null) => {
    if (node !== null) setInputId(node.id);
  }, []);
  return (
    <Field data-invalid={searchError !== null || undefined}>
      <FieldLabel id={labelId} htmlFor={inputId}>
        {label}
      </FieldLabel>
      <Command
        layout="inline"
        label={label}
        labelledBy={labelId}
        placeholder={null}
        inputRef={inputRef}
        icon={<Search aria-hidden="true" />}
        chosenId={chosenId}
        emptyText={searching || searchError !== null ? undefined : emptyText}
        value={value}
        onValueChange={onValueChange}
        choices={choices}
        exactChoice={exactChoice}
        onSelect={onSelect}
        frameClassName="w-full"
        busy={searching}
        describedBy={searchError !== null ? searchErrorId : undefined}
      />
      {searchError !== null && (
        <FieldError id={searchErrorId} role="status" aria-live="polite">
          {searchError}
        </FieldError>
      )}
    </Field>
  );
}

/** MergeSummary は窓の下端の「統合元 → 統合先」である（ui-design.md「Merge dialog」）。 */
export function MergeSummary({ source, target }: { source: string; target: string }) {
  return (
    <p className="flex min-w-0 items-center gap-1.5 text-sm text-muted-foreground">
      <span className="min-w-0 truncate">{source}</span>
      <ArrowRight aria-hidden="true" className="size-4 shrink-0" />
      <span className="sr-only"> → </span>
      <span className="min-w-0 truncate font-medium text-foreground">{target}</span>
    </p>
  );
}
