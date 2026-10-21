import { ArrowRight, Search } from "lucide-react";
import { type ComponentType, type ReactNode, useCallback, useId, useState } from "react";

import type { UiText } from "../i18n";
import type { NameChoice, NameCommandLayout } from "../ui/NameCommand";
import { Field, FieldError, FieldLabel } from "../ui/shadcn/field";

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
  hideEmptyPanel: boolean;
}

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
  searchError: UiText | null;
  /** Shown when no choice matches; without it an empty list draws no panel at all. */
  emptyText?: UiText;
}) {
  const searchErrorId = useId();
  const labelId = useId();
  // cmdk owns the input ID; read it from the rendered input to connect the visible label.
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
        hideEmptyPanel={emptyText === undefined}
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
