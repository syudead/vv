import type { Person } from "../../api/credits";
import { type CreditState, useCreditRecord, useRoutedCredit } from "../useCreditRecord";
import { useHandedPerson } from "./personHandoff";

export { parseCreditId as parsePersonId } from "../useCreditRecord";

export type PersonState = CreditState<Person>;

/**
 * Read one person, or start from `handed` without reading; any action that meets a missing person
 * calls markGone.
 */
export function usePerson(id: number | null, handed?: Person) {
  return useCreditRecord<Person>("person", id, handed);
}

/** The routed person, handed over or read, titling the document while it is shown. */
export function useRoutedPerson(id: number | null, title: (person: Person) => string) {
  const { record, ...result } = useRoutedCredit<Person>(
    "person",
    id,
    useHandedPerson(id),
    title,
  );
  return { ...result, person: record };
}
