import { useLocation } from "react-router";

import { t } from "../i18n";
import { ListPage } from "../ui/patterns/list-page";
import { FolderHeader } from "./Breadcrumbs";
import { FOLDERS_ROOT, folderKey, parseFolderPathname } from "./folderPath";
import FolderView from "./FolderView";
import RootView from "./RootView";
import { FolderNotFound } from "./states";

/**
 * FolderPage はフォルダ画面である。URL（`/folders`・`/folders/{rootId}/{段}…`）
 * から場所を読み、フォルダごとに中身を作り直す（key で状態を分ける）。
 *
 * 画面は責務ごとに分かれている: 最上位の登録フォルダと検索結果は `RootView`・
 * `RootSearchResults`、フォルダ1件の表示は `FolderView`（検索結果は
 * `FolderSearchResults`、直下の表示は `FolderContents`）、一覧の条件の管理は
 * `useConditions` が持ち、このファイルは URL から選んで組み立てるだけである。
 * どれもデザインシステムの一覧ページ（`ListPage`）で、見出しは見た目に出さない題と
 * パンくずの帯、ツールバーはトップバーの中、その下に本体を並べる
 * （web/registry/rules/patterns.md の List page）。
 */
export default function FolderPage() {
  const location = useLocation();
  const target = parseFolderPathname(location.pathname);

  return target.kind === "root" ? (
    <RootView />
  ) : target.kind === "folder" ? (
    <FolderView key={folderKey(target.folder)} folder={target.folder} />
  ) : (
    <ListPage
      toolbarPlacement="topBar"
      header={
        <FolderHeader
          title={t.folders.title}
          crumbs={[{ label: t.folders.title, to: FOLDERS_ROOT }, { label: "…" }]}
        />
      }
    >
      <FolderNotFound />
    </ListPage>
  );
}
