import { useCallback, useEffect, useRef } from "react";
import { useNavigate } from "react-router";

import type { GroupRef } from "../api/groups";
import type { UiText } from "../i18n";
import { useToast } from "../ui/Toast";
import { parentPagePath } from "./groupPaths";

/**
 * useLeaveGroup はグループが終わった（解散、タグ化、最後の項目を外した）後に画面を離れる
 * 操作を返す（specs/048-groups/ui-design.md「Dissolving」）。`message` があればトーストを出し、
 * 道筋の親のページ、無ければライブラリへ移る。画面を離れた後に答えた要求は、移った先の画面を
 * 動かさない。
 */
export function useLeaveGroup(trail: readonly GroupRef[]) {
  const navigate = useNavigate();
  const toast = useToast();
  /** 画面が出ている間だけ true。 */
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
      if (shown.current) void navigate(parentPagePath(trail));
    },
    [navigate, toast, trail],
  );
}
