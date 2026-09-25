import { createContext, type ReactNode, useContext } from "react";

/**
 * Audience は画面を描いている相手である。owner はログイン済みの所有者、guest は
 * 未ログインの人（specs/016-single-account-auth/plan.md Structural Decisions 2）。
 */
export type Audience = "owner" | "guest";

// 既定はゲストにする。ゲートの外で描いたときに所有者の操作を出さないためで、
// サーバーの Audience のゼロ値と同じ向きである。
const AudienceContext = createContext<Audience>("guest");

/** useAudience は今描いている相手を返す。ゲートが確かめた答えである。 */
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
