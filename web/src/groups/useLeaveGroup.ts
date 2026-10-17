import { useCallback, useEffect, useRef } from "react";
import { useNavigate } from "react-router";

import type { GroupRef } from "../api/groups";
import type { UiText } from "../i18n";
import { useToast } from "../ui/Toast";
import { parentPagePath } from "./groupPaths";

/**
 * useLeavePage は開いているものが終わった後に `path` へ画面を離れる操作を返す。`message` が
 * あればトーストを出す（移った先の画面で見える）。画面を離れた後に答えた要求は、移った先の
 * 画面を動かさない。グループのページとスマートフォルダのページ（削除）が共有する。
 */
export function useLeavePage(path: string) {
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
      if (shown.current) void navigate(path);
    },
    [navigate, path, toast],
  );
}

/**
 * useLeaveGroup はグループが終わった（解散、タグ化、最後の項目を外した）後に画面を離れる
 * 操作を返す。`message` があればトーストを出し、
 * 道筋の親のページ、無ければライブラリへ移る（useLeavePage）。
 */
export function useLeaveGroup(trail: readonly GroupRef[]) {
  return useLeavePage(parentPagePath(trail));
}
