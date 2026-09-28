import { FolderX } from "lucide-react";
import type { ReactNode } from "react";
import { Link } from "react-router";

import { formatNumber, t, type UiText } from "../i18n";
import { buttonClassName } from "../ui/Button";
import { EmptyState } from "../videoList/states";
import { FOLDERS_ROOT } from "./folderPath";

/** Section は見出し付きの一群である。見出しの件数も読み上げる。 */
export function Section({
  title,
  count,
  action,
  className,
  children,
}: {
  title: UiText;
  count?: number;
  /** 見出しの行の右端に置く操作（フォルダ画面の「Folder grouping menu」）。 */
  action?: ReactNode;
  className?: string;
  children: ReactNode;
}) {
  const heading = (
    <h2 className="px-0.5 text-xs font-semibold text-fg-muted">
      {title}
      {count !== undefined && (
        // 読み上げで名前と件数が続けて読まれないよう、余白ではなく空白で区切る。
        <span className="tabular-nums"> {formatNumber(count)}</span>
      )}
    </h2>
  );
  return (
    <section className={"flex flex-col gap-2 " + (className ?? "")}>
      {action === undefined ? (
        heading
      ) : (
        <div className="flex items-center justify-between gap-2">
          {heading}
          {action}
        </div>
      )}
      {children}
    </section>
  );
}

export function FolderNotFound() {
  return (
    <EmptyState
      icon={FolderX}
      title={t.folders.notFound.title}
      description={t.folders.notFound.description}
      action={
        <Link to={FOLDERS_ROOT} className={buttonClassName()}>
          {t.folders.notFound.back}
        </Link>
      }
    />
  );
}
