/** The integrated home and library share one URL and query state. */
export const LIBRARY_PATH = "/";

export function libraryFilterPath(param: string, id: number): string {
  return `${LIBRARY_PATH}?${param}=${String(id)}`;
}
