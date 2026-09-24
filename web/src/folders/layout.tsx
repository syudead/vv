import { FolderX } from "lucide-react";
import type { CSSProperties, ReactNode } from "react";
import { Link } from "react-router";

import type { Zoom } from "../preferences/viewPreferences";
import { buttonClassName } from "../ui/Button";
import { EmptyState } from "../videoList/states";
import { FOLDERS_ROOT } from "./folderPath";

const cardWidth: Record<Zoom, string> = {
  0: "var(--spacing-card-0)",
  1: "var(--spacing-card-1)",
  2: "var(--spacing-card-2)",
  3: "var(--spacing-card-3)",
};

/** Grid はライブラリと同じ格子（同じ幅・同じ間隔）である。 */
export function Grid({ zoom, children }: { zoom: Zoom; children: ReactNode }) {
  return (
    <div
      className="flex flex-wrap justify-center gap-2.5 [&>*]:w-[min(var(--card),100%)]"
      style={{ "--card": cardWidth[zoom] } as CSSProperties}
    >
      {children}
    </div>
  );
}

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

/** rangeLabel は一致なしのチップに添える範囲の名前である（ui-design.md「No-match state」）。 */
export function rangeLabel(kind: "subtree" | "direct", name: string): string {
  return kind === "subtree" ? `${name}とその中` : `${name}の直下`;
}
