import { screen, within } from "@testing-library/react";
import type { UserEvent } from "@testing-library/user-event";

import type { VideoEditSummary } from "../api/videoEdits";

/** A summary with only the tag and visibility counts page tests look at. */
export function editSummary(
  total: number,
  tags: VideoEditSummary["tags"] = [],
): VideoEditSummary {
  return { total, public: 0, tags, people: [], organizations: [], dates: [] };
}

/** Open the bulk editor from the selection bar once the selection's summary has arrived. */
export async function openBulkEditor(user: UserEvent): Promise<HTMLElement> {
  const bar = screen.getByRole("region", { name: "Selection actions" });
  await user.click(within(bar).getByRole("button", { name: "Edit" }));
  const dialog = await screen.findByRole("dialog", { name: "Edit videos" });
  await within(dialog).findByRole("combobox", { name: "Add a tag" });
  return dialog;
}

/** Add an existing tag to the selection through the bulk editor and apply it. */
export async function addTagInBulkEditor(user: UserEvent, name: string): Promise<void> {
  const dialog = await openBulkEditor(user);
  await user.type(within(dialog).getByRole("combobox", { name: "Add a tag" }), name);
  await user.click(await screen.findByRole("option", { name: new RegExp(name) }));
  await user.click(within(dialog).getByRole("button", { name: /^Apply to / }));
}

/** Remove a tag every selected video carries manually through the bulk editor and apply it. */
export async function removeTagInBulkEditor(
  user: UserEvent,
  name: string,
): Promise<void> {
  const dialog = await openBulkEditor(user);
  const tags = within(dialog).getByRole("list", { name: "Tags" });
  await user.click(
    within(tags).getByRole("button", { name: new RegExp(`^${name}, on `) }),
  );
  await user.click(within(dialog).getByRole("button", { name: /^Apply to / }));
}
