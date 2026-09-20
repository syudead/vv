import type { ReactNode } from "react";

import TopBar from "./TopBar";

/** AppShell は上部ナビバーを被せ、中身をその下に置く。 */
export default function AppShell({ children }: { children: ReactNode }) {
  return (
    <div className="min-h-dvh bg-bg">
      <TopBar />
      <main className="min-h-dvh pt-navbar">{children}</main>
    </div>
  );
}
