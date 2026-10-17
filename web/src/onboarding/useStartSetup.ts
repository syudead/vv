import { useCallback, useState } from "react";
import { useNavigate } from "react-router";

import { writeOnboarding } from "../api/onboarding";
import { errorText, t } from "../i18n";
import { useToast } from "../ui/Toast";
import { ONBOARDING_PATH } from "./onboardingRoute";

/**
 * useStartSetup はオンボードを始め直す操作である。段階を `folders` に書いてから `/welcome` を
 * 開く（docs/design-docs/home.md「Sidebar and library」）。段階を書かずに開くと、段階が
 * `done` の所有者はゲートがホームへ戻す。書けなかったら理由を通知に出して留まる。
 */
export function useStartSetup() {
  const navigate = useNavigate();
  const toast = useToast();
  const [pending, setPending] = useState(false);
  const start = useCallback(async () => {
    setPending(true);
    try {
      await writeOnboarding("folders");
      void navigate(ONBOARDING_PATH);
    } catch (error) {
      toast(t.onboarding.saveFailed(errorText(error)));
    } finally {
      setPending(false);
    }
  }, [navigate, toast]);
  return { start, pending };
}
