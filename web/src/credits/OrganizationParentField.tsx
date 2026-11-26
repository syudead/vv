import { useEffect, useState } from "react";

import { isAborted } from "../api/client";
import { type Credit, listCredits, type Organization } from "../api/credits";
import { errorText, t, type UiText } from "../i18n";
import { controlId, type FieldProblem, Problem } from "../player/detailsEditor/fields";
import { Combobox, type ComboboxOption } from "../ui/shadcn/combobox";
import { Field, FieldError, FieldTitle } from "../ui/shadcn/field";

/** Combobox values cannot be empty, so None needs a sentinel. */
export const noParent = "none";

/** The select's value for the organization's current parent. */
export function parentValueOf(organization: Pick<Organization, "parent">): string {
  return organization.parent === undefined ? noParent : String(organization.parent.id);
}

/** The patch value of a select value: null for None. */
export function parentIdOf(value: string): number | null {
  return value === noParent ? null : Number(value);
}

/**
 * Load all other organizations as parent candidates; until loaded, or when the read fails, show
 * only the current parent and disable selection.
 */
export function OrganizationParentField({
  organization,
  value,
  onChange,
  disabled,
  problem,
  title,
}: {
  organization: Pick<Organization, "id" | "parent">;
  value: string;
  onChange: (value: string) => void;
  disabled: boolean;
  problem: FieldProblem | null;
  /** A visible label above the select, where no legend names it. */
  title?: UiText;
}) {
  const d = t.credits.details;
  const [organizations, setOrganizations] = useState<readonly Credit[] | null>(null);
  const [failure, setFailure] = useState<UiText | null>(null);

  useEffect(() => {
    const controller = new AbortController();
    listCredits("organization", {}, controller.signal).then(
      (page) => {
        if (!controller.signal.aborted) setOrganizations(page.items);
      },
      (error: unknown) => {
        if (controller.signal.aborted || isAborted(error)) return;
        setFailure(d.parentsFailed(errorText(error)));
      },
    );
    return () => controller.abort();
  }, [d]);

  const current = organization.parent;
  const others =
    organizations ??
    (current === undefined ? [] : [{ id: current.id, name: current.name }]);
  const options: ComboboxOption[] = [
    { value: noParent, label: d.parentNone },
    ...others
      .filter((other) => other.id !== organization.id)
      .map((other) => ({ value: String(other.id), label: other.name })),
  ];

  return (
    <Field data-invalid={problem?.control === "parent" || undefined}>
      {title !== undefined && <FieldTitle>{title}</FieldTitle>}
      <div id={controlId("parent")}>
        <Combobox
          options={options}
          value={value}
          onValueChange={onChange}
          placeholder={d.parentNone}
          searchPlaceholder={d.parentSearch}
          emptyText={d.parentEmpty}
          disabled={disabled || organizations === null}
          aria-label={d.parent}
          className="w-full"
        />
      </div>
      <Problem problem={problem} control="parent" />
      {failure !== null && <FieldError>{failure}</FieldError>}
    </Field>
  );
}
