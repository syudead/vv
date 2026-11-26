import type { CreditPatch, Organization } from "../../../api/credits";
import type { IdRow } from "../../../player/detailsEditor/draft";
import { idRowsOf, namesAndIdsPatch, type Sent } from "../../creditEditFields";
import { type NameRow, nameRowsOf } from "../../NameRows";
import { parentIdOf, parentValueOf } from "../../OrganizationParentField";
import {
  type OrganizationDraft,
  organizationDraftOf,
  organizationProfilePatch,
} from "../../OrganizationProfileFields";

/** Every input of the edit page as typed. */
export interface OrganizationEditDraft {
  /** The names in list order; the first row is the display name. */
  names: NameRow[];
  externalIds: IdRow[];
  /** The parent select's value. */
  parent: string;
  profile: OrganizationDraft;
}

export function draftFromOrganization(organization: Organization): OrganizationEditDraft {
  return {
    names: nameRowsOf(organization.names),
    externalIds: idRowsOf(organization.externalIds),
    parent: parentValueOf(organization),
    profile: organizationDraftOf(organization.profile),
  };
}

/**
 * The PATCH body with only the properties that differ from the loaded organization, so a
 * concurrent change to another property survives; edited lists replace their whole value.
 */
export function buildOrganizationPatch(
  draft: OrganizationEditDraft,
  loaded: OrganizationEditDraft,
): { patch: CreditPatch; sent: Sent } {
  const { patch, sent } = namesAndIdsPatch(draft, loaded);
  if (draft.parent !== loaded.parent) patch.parentId = parentIdOf(draft.parent);
  const profile = organizationProfilePatch(draft.profile, loaded.profile);
  if (Object.keys(profile).length > 0) patch.organizationProfile = profile;
  return { patch, sent };
}

export function organizationDraftChanged(
  draft: OrganizationEditDraft,
  loaded: OrganizationEditDraft,
): boolean {
  return Object.keys(buildOrganizationPatch(draft, loaded).patch).length > 0;
}
