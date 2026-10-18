import type { ReactNode } from "react";

import { AudienceProvider } from "../auth/audience";

/** Tests need this wrapper because rendering outside AuthGate defaults to guest. */
export function OwnerAudience({ children }: { children: ReactNode }) {
  return <AudienceProvider audience="owner">{children}</AudienceProvider>;
}
