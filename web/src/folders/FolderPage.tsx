import { useLocation } from "react-router";

import { t } from "../i18n";
import { ListPage } from "../ui/patterns/list-page";
import { PageHeader } from "../ui/patterns/page-header";
import Breadcrumbs from "./Breadcrumbs";
import { FOLDERS_ROOT, folderKey, parseFolderPathname } from "./folderPath";
import FolderView from "./FolderView";
import RootView from "./RootView";
import { FolderNotFound } from "./states";

/** Key folder contents by URL location so state cannot leak between folders. */
export default function FolderPage() {
  const location = useLocation();
  const target = parseFolderPathname(location.pathname);

  return target.kind === "root" ? (
    <RootView />
  ) : target.kind === "folder" ? (
    <FolderView key={folderKey(target.folder)} folder={target.folder} />
  ) : (
    <ListPage
      header={
        <>
          <Breadcrumbs
            crumbs={[{ label: t.folders.title, to: FOLDERS_ROOT }, { label: "…" }]}
          />
          <PageHeader titleHidden title={t.folders.title} />
        </>
      }
    >
      <FolderNotFound />
    </ListPage>
  );
}
