import { X } from "lucide-react";
import { Fragment, useId } from "react";

import type { CreditKind } from "../../api/credits";
import {
  type WorkOrganizationRole,
  type WorkPersonRole,
  workOrganizationRoles,
  workPersonRoles,
} from "../../api/workInfo";
import { creditFace } from "../../credits/creditFace";
import { t, type UiText } from "../../i18n";
import { cn } from "../../lib/cn";
import { Badge } from "../../ui/shadcn/badge";
import { Button } from "../../ui/shadcn/button";
import { FieldLegend, FieldSet, FieldTitle } from "../../ui/shadcn/field";
import type { PanelProps } from "./BasicsPanel";
import CreditPicker, { type UnsavedName } from "./CreditPicker";
import {
  type Chip,
  chipLabel,
  chipsOf,
  type Draft,
  isNewChip,
  newEntries,
  type Role,
  withChips,
} from "./draft";
import { controlId, invalidProps, Problem } from "./fields";

export function personRole(role: WorkPersonRole): { label: UiText; add: UiText } {
  const e = t.player.detailsEditor;
  if (role === "performer") return { label: e.performers, add: e.addPerformer };
  if (role === "director") return { label: e.directors, add: e.addDirector };
  if (role === "photographer") return { label: e.photographers, add: e.addPhotographer };
  return { label: e.streamers, add: e.addStreamer };
}

export function organizationRole(role: WorkOrganizationRole): {
  label: UiText;
  add: UiText;
} {
  const e = t.player.detailsEditor;
  return role === "maker"
    ? { label: e.makers, add: e.addMaker }
    : { label: e.labels, add: e.addLabel };
}

function unsavedOf(draft: Draft, kind: CreditKind): UnsavedName[] {
  return newEntries(draft).filter((entry) => entry.kind === kind);
}

export default function CreditsPanel(props: PanelProps) {
  const e = t.player.detailsEditor;
  return (
    <div className="flex flex-col gap-6">
      <RoleGroup
        legend={e.people}
        roles={workPersonRoles.map((role) => ({
          role: { kind: "person", role },
          ...personRole(role),
        }))}
        {...props}
      />
      <RoleGroup
        legend={e.organizations}
        roles={workOrganizationRoles.map((role) => ({
          role: { kind: "organization", role },
          ...organizationRole(role),
        }))}
        {...props}
      />
    </div>
  );
}

function RoleGroup({
  legend,
  roles,
  ...props
}: PanelProps & {
  legend: UiText;
  roles: { role: Role; label: UiText; add: UiText }[];
}) {
  return (
    <FieldSet className="gap-0">
      <FieldLegend variant="label">{legend}</FieldLegend>
      <div className="grid grid-cols-term items-start gap-x-4 gap-y-3">
        {roles.map((entry) => (
          <RoleRow key={entry.role.role} {...entry} {...props} />
        ))}
      </div>
    </FieldSet>
  );
}

function RoleRow({
  role,
  label,
  add,
  draft,
  update,
  saving,
  problem,
}: PanelProps & { role: Role; label: UiText; add: UiText }) {
  const labelId = useId();
  const chips = chipsOf(draft, role);
  const remove = (chip: Chip) => {
    if (saving) return;
    update((current) =>
      withChips(
        current,
        role,
        chipsOf(current, role).filter((item) => item.key !== chip.key),
      ),
    );
  };
  return (
    <Fragment>
      <FieldTitle id={labelId} className="pt-1.5 whitespace-nowrap">
        {label}
      </FieldTitle>
      <div className="flex min-w-0 flex-col gap-1">
        <ul
          aria-labelledby={labelId}
          className="flex min-w-0 flex-wrap items-center gap-1.5"
        >
          {chips.map((chip) => (
            <CreditChip
              key={chip.key}
              kind={role.kind}
              chip={chip}
              saving={saving}
              problem={problem}
              onRemove={() => remove(chip)}
            />
          ))}
          <li className="max-w-full min-w-0">
            <CreditPicker
              kind={role.kind}
              offerUnnamed={role.kind === "person" && role.role === "performer"}
              label={add}
              chips={chips}
              unsaved={unsavedOf(draft, role.kind)}
              disabled={saving}
              onAdd={(chip) =>
                update((current) =>
                  withChips(current, role, [...chipsOf(current, role), chip]),
                )
              }
            />
          </li>
        </ul>
        {chips.map((chip) => (
          <Problem key={chip.key} problem={problem} control={`chip-${chip.key}`} />
        ))}
      </div>
    </Fragment>
  );
}

function CreditChip({
  kind,
  chip,
  saving,
  problem,
  onRemove,
}: {
  kind: CreditKind;
  chip: Chip;
  saving: boolean;
  problem: PanelProps["problem"];
  onRemove: () => void;
}) {
  const label = chipLabel(chip);
  const unnamed = !isNewChip(chip) && chip.provisional === true;
  return (
    <li className="max-w-full min-w-0">
      <Badge
        variant="secondary"
        title={label}
        className={cn(
          "h-auto min-h-6 max-w-full justify-start gap-0 pr-0 font-normal whitespace-normal",
          kind === "person" && "pl-0",
        )}
      >
        {kind === "person" && (
          <span className="mr-1.5 flex shrink-0">
            {creditFace(kind, isNewChip(chip) ? undefined : chip, "token")}
          </span>
        )}
        <span className={cn("min-w-0 wrap-anywhere", unnamed && "text-muted-foreground")}>
          {label}
        </span>
        {isNewChip(chip) && (
          <span className="ml-1.5 shrink-0 text-xs text-muted-foreground">
            {t.player.detailsEditor.newMark}
          </span>
        )}
        <span aria-hidden="true" className="ml-1.5 h-4 w-px shrink-0 bg-input" />
        <Button
          id={controlId(`chip-${chip.key}`)}
          variant="ghost"
          size="icon-sm"
          aria-label={t.player.detailsEditor.remove(label)}
          aria-disabled={saving || undefined}
          onClick={onRemove}
          className="size-6 rounded-l-none rounded-r-sm [&_svg]:size-3"
          {...invalidProps(problem, `chip-${chip.key}`)}
        >
          <X aria-hidden="true" />
        </Button>
      </Badge>
    </li>
  );
}
