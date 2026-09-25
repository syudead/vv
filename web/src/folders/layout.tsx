import { FolderX } from "lucide-react";
import type { ReactNode } from "react";
import { Link } from "react-router";

import { buttonClassName } from "../ui/Button";
import { EmptyState } from "../videoList/states";
import { FOLDERS_ROOT } from "./folderPath";

/** Section は見出し付きの一群である。見出しの件数も読み上げる。 */
export function Section({
  title,
  count,
  className,
  children,
}: {
  title: string;
  count?: number;
  className?: string;
  children: ReactNode;
}) {
  return (
    <section className={"flex flex-col gap-2 " + (className ?? "")}>
      <h2 className="px-0.5 text-xs font-semibold text-fg-muted">
        {title}
        {count !== undefined && (
          // 読み上げで名前と件数が続けて読まれないよう、余白ではなく空白で区切る。
          <span className="tabular-nums"> {count.toLocaleString("ja-JP")}</span>
        )}
      </h2>
      {children}
    </section>
  );
}

export function FolderNotFound() {
  return (
    <EmptyState
      icon={FolderX}
      title="このフォルダは見つかりません"
      description="登録が外れたか、中の動画が無くなりました。"
      action={
        <Link to={FOLDERS_ROOT} className={buttonClassName()}>
          フォルダの一覧へ
        </Link>
      }
    />
  );
}
