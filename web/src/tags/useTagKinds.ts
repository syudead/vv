import {
  type Dispatch,
  type SetStateAction,
  useCallback,
  useEffect,
  useState,
} from "react";

import {
  currentTagKinds,
  refreshTagKinds,
  sortTagKinds,
  subscribeTagKinds,
  type TagKind,
} from "../api/tagKinds";
import { errorText, type UiText } from "../i18n";

export interface TagKindList {
  kinds: TagKind[] | undefined;
  /** Cached data alone cannot prove a URL kind is gone; require a successful read since mount. */
  fresh: boolean;
  loadError: UiText | null;
  setKinds: Dispatch<SetStateAction<TagKind[] | undefined>>;
  reload: () => void;
}

/** The kinds in name order, refreshed on mount even when cached because their tag counts change. */
export function useTagKinds(): TagKindList {
  const [kinds, setKinds] = useState<TagKind[] | undefined>(() => {
    const held = currentTagKinds();
    return held === undefined ? undefined : sortTagKinds(held);
  });
  const [loadError, setLoadError] = useState<UiText | null>(null);
  const [fresh, setFresh] = useState(false);

  useEffect(() => {
    const received = (list: TagKind[]) => {
      setKinds(sortTagKinds(list));
      setLoadError(null);
      setFresh(true);
    };
    const unsubscribe = subscribeTagKinds(received);
    refreshTagKinds().then(received, (failure: unknown) =>
      setLoadError(errorText(failure)),
    );
    return unsubscribe;
  }, []);

  /** Keep the rows during a refresh, but expose a failure instead of stale counts. */
  const reload = useCallback(() => {
    setLoadError(null);
    refreshTagKinds().catch((failure: unknown) => setLoadError(errorText(failure)));
  }, []);

  return { kinds, fresh, loadError, setKinds, reload };
}
