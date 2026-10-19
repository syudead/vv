/** The person page, `/people/:id`. */
export function personPath(id: number): string {
  return `/people/${String(id)}`;
}
