import type { ReactNode } from "react";

import { AudienceProvider } from "../auth/audience";

/**
 * OwnerAudience はテストで所有者として描くための包みである。ゲートの外の既定は
 * ゲスト（auth/audience.tsx）なので、所有者の画面を確かめるテストはこれで包む。
 */
export function OwnerAudience({ children }: { children: ReactNode }) {
  return <AudienceProvider audience="owner">{children}</AudienceProvider>;
}
