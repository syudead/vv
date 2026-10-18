import { useCallback, useEffect, useRef } from "react";
import { useNavigate } from "react-router";

import type { GroupRef } from "../api/groups";
import type { UiText } from "../i18n";
import { useToast } from "../ui/Toast";
import { parentPagePath } from "./groupPaths";

/**
 * Ignore responses after leaving so they cannot navigate a different page; notifications survive
 * navigation.
 */
export function useLeavePage(path: string) {
  const navigate = useNavigate();
  const toast = useToast();
  const shown = useRef(true);
  useEffect(() => {
    shown.current = true;
    return () => {
      shown.current = false;
    };
  }, []);

  return useCallback(
    (message?: UiText) => {
      if (message !== undefined) toast(message);
      if (shown.current) void navigate(path);
    },
    [navigate, path, toast],
  );
}

export function useLeaveGroup(trail: readonly GroupRef[]) {
  return useLeavePage(parentPagePath(trail));
}
