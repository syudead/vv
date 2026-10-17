import { Navigate, useLocation } from "react-router";

import { LIBRARY_PATH } from "../library/libraryPath";

/**
 * LibraryRedirect はトップ（`/`）を、問い合わせと `#` をそのままにライブラリ（`/library`）へ
 * 送る（specs/057-home/research.md R-15）。`/?tag=3` などのブックマークとリンクが絞り込み
 * ごとライブラリを開く。問い合わせの無い `/` はホームで、HomeRoute がここへは送らない。
 */
export default function LibraryRedirect() {
  const { search, hash } = useLocation();
  return <Navigate replace to={{ pathname: LIBRARY_PATH, search, hash }} />;
}
