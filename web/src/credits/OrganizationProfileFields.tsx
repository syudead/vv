import type { KeyboardEvent } from "react";

import type { OrganizationProfile, OrganizationProfilePatch } from "../api/credits";
import { t, type UiText } from "../i18n";
import {
  controlId,
  type FieldProblem,
  invalidProps,
  Problem,
} from "../player/detailsEditor/fields";
import { Field, FieldDescription, FieldLabel } from "../ui/shadcn/field";
import { Input } from "../ui/shadcn/input";
import { Textarea } from "../ui/shadcn/textarea";

/** The editable profile as typed; links are one URL per line. */
export interface OrganizationDraft {
  foundedDate: string;
  dissolvedDate: string;
  location: string;
  bio: string;
  links: string;
}

const textFields = ["foundedDate", "dissolvedDate", "location", "bio"] as const;

export function organizationDraftOf(profile: OrganizationProfile): OrganizationDraft {
  return {
    foundedDate: profile.foundedDate ?? "",
    dissolvedDate: profile.dissolvedDate ?? "",
    location: profile.location ?? "",
    bio: profile.bio ?? "",
    links: (profile.links ?? []).join("\n"),
  };
}

function linksOf(text: string): string[] {
  return text
    .split("\n")
    .map((line) => line.trim())
    .filter((line) => line !== "");
}

/**
 * The properties that differ from the loaded profile, so a concurrent change to another one
 * survives; an emptied field is sent as null to clear it, and changed links replace the list.
 */
export function organizationProfilePatch(
  draft: OrganizationDraft,
  loaded: OrganizationDraft,
): OrganizationProfilePatch {
  const patch: OrganizationProfilePatch = {};
  for (const field of textFields) {
    const value = draft[field].trim();
    if (value !== loaded[field].trim()) patch[field] = value === "" ? null : value;
  }
  const links = linksOf(draft.links);
  if (links.join("\n") !== linksOf(loaded.links).join("\n")) patch.links = links;
  return patch;
}

/** The control a server field such as organizationProfile.links[2] names, or undefined. */
export function organizationProfileControl(field: string): string | undefined {
  const match = /^organizationProfile\.(\w+)/.exec(field);
  return match === null ? undefined : `organization-${match[1]!}`;
}

export function OrganizationProfileFields({
  draft,
  onChange,
  saving,
  problem,
  onEnter,
}: {
  draft: OrganizationDraft;
  onChange: (next: OrganizationDraft) => void;
  saving: boolean;
  problem: FieldProblem | null;
  onEnter: (event: KeyboardEvent<HTMLInputElement>) => void;
}) {
  const d = t.credits.details;
  const set = (field: keyof OrganizationDraft, value: string) =>
    onChange({ ...draft, [field]: value });
  const line = (field: "foundedDate" | "dissolvedDate" | "location", label: UiText) => {
    const control = `organization-${field}`;
    return (
      <Field data-invalid={problem?.control === control || undefined}>
        <FieldLabel htmlFor={controlId(control)}>{label}</FieldLabel>
        <Input
          id={controlId(control)}
          value={draft[field]}
          readOnly={saving}
          placeholder={field === "location" ? undefined : d.datePlaceholder}
          onChange={(event) => set(field, event.target.value)}
          onKeyDown={onEnter}
          className="h-8"
          {...invalidProps(problem, control)}
        />
        <Problem problem={problem} control={control} />
      </Field>
    );
  };
  return (
    <>
      <div className="grid gap-3 sm:grid-cols-2">
        {line("foundedDate", d.foundedDate)}
        {line("dissolvedDate", d.dissolvedDate)}
      </div>
      {line("location", d.location)}
      <Field data-invalid={problem?.control === "organization-bio" || undefined}>
        <FieldLabel htmlFor={controlId("organization-bio")}>{d.bio}</FieldLabel>
        <Textarea
          id={controlId("organization-bio")}
          value={draft.bio}
          readOnly={saving}
          onChange={(event) => set("bio", event.target.value)}
          rows={3}
          {...invalidProps(problem, "organization-bio")}
        />
        <Problem problem={problem} control="organization-bio" />
      </Field>
      <Field data-invalid={problem?.control === "organization-links" || undefined}>
        <FieldLabel htmlFor={controlId("organization-links")}>{d.links}</FieldLabel>
        <FieldDescription>{d.linksHint}</FieldDescription>
        <Textarea
          id={controlId("organization-links")}
          value={draft.links}
          readOnly={saving}
          onChange={(event) => set("links", event.target.value)}
          rows={2}
          {...invalidProps(problem, "organization-links")}
        />
        <Problem problem={problem} control="organization-links" />
      </Field>
    </>
  );
}
