import { useLocation } from "react-router";

import HomePage from "../home/HomePage";
import AppShell from "../shell/AppShell";
import LibraryRedirect from "./LibraryRedirect";

/**
 * HomeRoute はトップ（`/`）である。問い合わせの無い `/` はシェルの中のホームを出し、問い合わせの
 * 付いた `/?tag=3` などは前のトップのブックマークとリンクとして、問い合わせごとライブラリへ
 * 送る（specs/057-home/research.md R-15）。
 */
export default function HomeRoute() {
  const { search } = useLocation();
  if (search !== "") return <LibraryRedirect />;
  return (
    <AppShell>
      <HomePage />
    </AppShell>
  );
}
