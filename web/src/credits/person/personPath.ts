/** The person page, `/people/:id`. */
export function personPath(id: number): string {
  return `/people/${String(id)}`;
}

/** The person's face recognition page, `/people/:id/faces`. */
export function personFacesPath(id: number): string {
  return `/people/${String(id)}/faces`;
}

export function isPersonFacesPath(pathname: string): boolean {
  return /^\/people\/[^/]+\/faces\/?$/i.test(pathname);
}
