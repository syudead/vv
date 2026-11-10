import type { ReactNode } from "react";

import { BackgroundContext, type BackgroundSource } from "../shell/BackgroundProvider";

const pausedSource: BackgroundSource = {
  settings: { paused: true, running: [], groups: [] },
  loadError: null,
  load: () => Promise.resolve(),
  beginSave: () => undefined,
  saved: () => undefined,
  update: () => undefined,
  endSave: () => undefined,
};

/** A fixed paused status, for screens that only change their waiting texts while paused. */
export function PausedBackground({ children }: { children: ReactNode }) {
  return (
    <BackgroundContext.Provider value={pausedSource}>
      {children}
    </BackgroundContext.Provider>
  );
}
