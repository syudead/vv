/** The organization page, `/organizations/:id`. */
export function organizationPath(id: number): string {
  return `/organizations/${String(id)}`;
}

/** The organization's edit page, `/organizations/:id/edit`. */
export function organizationEditPath(id: number): string {
  return `${organizationPath(id)}/edit`;
}
