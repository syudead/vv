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

  const [checkedKey, setCheckedKey] = useState<string | null>(null);

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

    const unsubscribeServer = subscribeServerEvents({
      onboarding: () => void load(controller.signal),
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
