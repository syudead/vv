import { Navigate, useLocation } from "react-router";

import { LIBRARY_PATH } from "../library/libraryPath";

/** Preserve bookmarks from the former standalone library. */
export default function LibraryRedirect() {
  const { search, hash } = useLocation();
  return <Navigate replace to={{ pathname: LIBRARY_PATH, search, hash }} />;
}
