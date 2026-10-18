import { createContext, useContext } from "react";

/**
 * A null targetId means a newly created group; lists reload only when the mutation changes their
 * contents.
 */
export type GroupChanged = (targetId: number | null) => void;

function ignore(): void {
  // No-op outside list providers, such as on playback pages.
}

export const GroupChangedContext = createContext<GroupChanged>(ignore);

export function useGroupChanged(): GroupChanged {
  return useContext(GroupChangedContext);
}
