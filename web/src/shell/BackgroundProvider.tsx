import {
  createContext,
  type ReactNode,
  useContext,
  useEffect,
  useMemo,
  useRef,
} from "react";

import { type BackgroundWork, getBackgroundWork } from "../api/background";
import { subscribeServerEvents } from "../api/serverEvents";
import { useAudience } from "../auth/audience";
import { type SettingsSource, useSettingsSource } from "../settings/useSettingsSource";

/** The background work status: read once, then again on each `background` event and reconnect. */
export type BackgroundSource = SettingsSource<BackgroundWork>;

const BackgroundContext = createContext<BackgroundSource | null>(null);

/** Null for a guest and outside the provider; a guest's screens show no background work. */
export function useBackground(): BackgroundSource | null {
  return useContext(BackgroundContext);
}

function OwnerBackgroundProvider({ children }: { children: ReactNode }) {
  const { settings, loadError, load, beginSave, saved, update, endSave } =
    useSettingsSource(getBackgroundWork);
  // An event during a pause request is not read then; one read after the answer catches up.
  const missed = useRef(false);
  const saving = useRef(0);

  useEffect(() => {
    const reload = () => {
      if (saving.current > 0) {
        missed.current = true;
        return;
      }
      void load();
    };
    // The provider's first read covers the initial connection; a reconnect may have missed events.
    return subscribeServerEvents({
      background: reload,
      open: (reconnected) => {
        if (reconnected) reload();
      },
    });
  }, [load]);

  const value = useMemo<BackgroundSource>(
    () => ({
      settings,
      loadError,
      load,
      saved,
      update,
      beginSave: () => {
        saving.current += 1;
        beginSave();
      },
      endSave: () => {
        saving.current -= 1;
        endSave();
        if (saving.current === 0 && missed.current) {
          missed.current = false;
          void load();
        }
      },
    }),
    [beginSave, endSave, load, loadError, saved, settings, update],
  );

  return (
    <BackgroundContext.Provider value={value}>{children}</BackgroundContext.Provider>
  );
}

/** Sits beside ScanProvider; only the owner's screens read the status or subscribe to its event. */
export function BackgroundProvider({ children }: { children: ReactNode }) {
  if (useAudience() !== "owner") return children;
  return <OwnerBackgroundProvider>{children}</OwnerBackgroundProvider>;
}
