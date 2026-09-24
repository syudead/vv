import { useLocation } from "react-router";

import Breadcrumbs from "./Breadcrumbs";
import { FOLDERS_ROOT, folderKey, parseFolderPathname } from "./folderPath";
import FolderView from "./FolderView";
import { FolderNotFound } from "./layout";
import RootView from "./RootView";

/**
 * FolderPage はフォルダ画面である。URL（`/folders`・`/folders/{rootId}/{段}…`）
 * から場所を読み、フォルダごとに中身を作り直す（key で状態を分ける）。
 *
 * 画面は責務ごとに分かれている: 最上位の登録フォルダと検索結果は `RootView`・
 * `RootSearchResults`、フォルダ1件の表示は `FolderView`（検索結果は
 * `FolderSearchResults`、直下の表示は `FolderContents`）、一覧の条件の管理は
 * `useConditions` が持ち、このファイルは URL から選んで組み立てるだけである。
 */
export default function FolderPage() {
  const location = useLocation();
  const target = parseFolderPathname(location.pathname);

  return (
    <div className="flex w-full flex-col gap-3 px-3 pt-3 pb-24 sm:px-4">
      {target.kind === "root" ? (
        <RootView />
      ) : target.kind === "folder" ? (
        <FolderView key={folderKey(target.folder)} folder={target.folder} />
      ) : (
        <>
          <h1 className="sr-only">フォルダ</h1>
          <Breadcrumbs
            crumbs={[{ label: "フォルダ", to: FOLDERS_ROOT }, { label: "…" }]}
          />
          <FolderNotFound />
        </>
      )}
    </div>
  );
}
