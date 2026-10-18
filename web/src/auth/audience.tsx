import { createContext, type ReactNode, useContext } from "react";

export type Audience = "owner" | "guest";

// Default to guest so rendering outside the gate never exposes owner controls.
const AudienceContext = createContext<Audience>("guest");

export function useAudience(): Audience {
  return useContext(AudienceContext);
}

export function AudienceProvider({
  audience,
  children,
}: {
  audience: Audience;
  children: ReactNode;
}) {
  return <AudienceContext.Provider value={audience}>{children}</AudienceContext.Provider>;
}
