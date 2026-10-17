import {
  createContext,
  type ReactNode,
  useCallback,
  useContext,
  useEffect,
  useRef,
  useState,
} from "react";
import { Navigate, useLocation } from "react-router";

import { isAborted } from "../api/client";
import {
  type OnboardingStep,
  readOnboarding,
  subscribeOnboarding,
} from "../api/onboarding";
import { subscribeServerEvents } from "../api/serverEvents";
import { useAudience } from "../auth/audience";
import { routePath } from "../auth/AuthGate";
import { ONBOARDING_PATH, onboardingRedirect } from "./onboardingRoute";

// オンボードのゲート（docs/design-docs/home.md「Onboarding」、research.md R-3・R-4）。
// 所有者の段階を読み、`done` でない間はどの経路も `/welcome` へ置き換える。段階は `home` の
// 知らせとつなぎ直しで読み直し、ほかの端末での変化に合わせる。この画面から書いた段階は
// api/onboarding.ts の知らせで、知らせを待たずに当てる。

type GateState = { status: "loading" } | { status: "ready"; step: OnboardingStep };

const StepContext = createContext<OnboardingStep>("done");

/** useOnboardingStep はゲートが読んだ今の段階を返す。ゲートの外と閲覧者は `done`。 */
export function useOnboardingStep(): OnboardingStep {
  return useContext(StepContext);
}

/**
 * OnboardingGate は所有者の段階が分かるまで何も描かず、分かったら段階に合う経路へ置き換える。
 * 段階を読めなかったときは `done` と扱い、どの画面も止めない。閲覧者は読まずにそのまま描く。
 */
export function OnboardingGate({ children }: { children: ReactNode }) {
  const owner = useAudience() === "owner";
  const location = useLocation();
  const [state, setState] = useState<GateState>({ status: "loading" });
  // checkedKey は、段階が done のまま `/welcome` を開いたときに読み直した遷移の鍵である。
  // 設定の「Start setup」のように別の所から段階を書いてから来たときに、古い done で
  // ホームへ戻さない。
  const [checkedKey, setCheckedKey] = useState<string | null>(null);
  // sequence は最後に始めた読み込み（か書いた段階の反映）の通し番号である。古い応答で
  // 新しい段階を戻さない。
  const sequence = useRef(0);

  const load = useCallback(async (signal: AbortSignal) => {
    sequence.current += 1;
    const mine = sequence.current;
    try {
      const step = await readOnboarding(signal);
      if (mine === sequence.current) setState({ status: "ready", step });
    } catch (error) {
      if (isAborted(error) || mine !== sequence.current) return;
      setState((current) =>
        current.status === "ready" ? current : { status: "ready", step: "done" },
      );
    }
  }, []);

  useEffect(() => {
    if (!owner) return;
    const controller = new AbortController();
    // 知らせを先に購読してから読む。読んだ後の変化を取りこぼさない。
    const unsubscribeServer = subscribeServerEvents({
      home: () => void load(controller.signal),
      open: (reconnected) => {
        if (reconnected) void load(controller.signal);
      },
    });
    const unsubscribeWrites = subscribeOnboarding((step) => {
      sequence.current += 1;
      setState({ status: "ready", step });
    });
    void load(controller.signal);
    return () => {
      controller.abort();
      unsubscribeServer();
      unsubscribeWrites();
    };
  }, [load, owner]);

  const pathname = routePath(location.pathname);
  const checking =
    owner &&
    state.status === "ready" &&
    state.step === "done" &&
    pathname === ONBOARDING_PATH &&
    checkedKey !== location.key;

  useEffect(() => {
    if (!checking) return;
    const controller = new AbortController();
    const key = location.key;
    void load(controller.signal).then(() => {
      if (!controller.signal.aborted) setCheckedKey(key);
    });
    return () => controller.abort();
  }, [checking, load, location.key]);

  if (!owner) return children;
  if (state.status === "loading" || checking) return null;
  const target = onboardingRedirect(owner, state.step, pathname);
  if (target !== null) return <Navigate replace to={target} />;
  return <StepContext.Provider value={state.step}>{children}</StepContext.Provider>;
}
