import { useEffect, useState } from "react";

import type { Person } from "../../api/credits";

// One in-memory slot carries the person from the page to its editor and the saved person back,
// so neither screen reads it again; history state would go stale on reload. Peek during render
// (StrictMode renders twice) and clear in the receiving effect.

let handed: Person | null = null;

export function handPerson(person: Person): void {
  handed = person;
}

/** The person handed over for this id, if any; a direct URL gets undefined and reads it. */
export function useHandedPerson(id: number | null): Person | undefined {
  const [person] = useState(() =>
    handed !== null && handed.id === id ? handed : undefined,
  );
  useEffect(() => {
    handed = null;
  }, []);
  return person;
}
