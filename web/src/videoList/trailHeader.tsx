import { FolderX, Play } from "lucide-react";
import { Link } from "react-router";

import Breadcrumbs from "../folders/Breadcrumbs";
import type { Crumb } from "../folders/folderPath";
import { t, type UiText } from "../i18n";
import { LIBRARY_PATH } from "../library/libraryPath";
import { EmptyState } from "../ui/patterns/empty-state";
import { PageHeader } from "../ui/patterns/page-header";
import { Button } from "../ui/shadcn/button";
import { Skeleton } from "../ui/shadcn/skeleton";

// 1 つのものを開いた一覧の画面（グループのページ、スマートフォルダのページ）の、道筋の帯と
// 見出しが共有する部品。

/**
 * LoadingTrailHeader は読み込み中の帯（名前の分からない段は骨組み）と、題の代わりの骨組みで
 * ある。帯の読み上げ名は `label`。
 */
export function LoadingTrailHeader({
  label,
  crumbs,
}: {
  label: UiText;
  crumbs: (Crumb | undefined)[];
}) {
  return (
    <>
      <Breadcrumbs label={label} crumbs={crumbs} keepFirst />
      <PageHeader
        title={
          <>
            <span className="sr-only">{t.list.loading}</span>
            <Skeleton aria-hidden="true" className="h-6 w-1/2" />
          </>
        }
      />
    </>
  );
}

/**
 * UnavailableTrailHeader は読めなかったときと無くなったときの帯と見出しである。見せられない
 * ものの名前を出さず、「Library」だけにする。
 */
export function UnavailableTrailHeader({ label }: { label: UiText }) {
  return (
    <>
      <Breadcrumbs label={label} crumbs={[{ label: t.library.title }]} keepFirst />
      <PageHeader title={t.library.title} />
    </>
  );
}

/**
 * PlayLink は見出しの「Play」である。`to` の動画を自動再生で開き、戻る先を `from` にする。
 * 読み上げ名は「Play "{name}"」。
 */
export function PlayLink({ to, from, name }: { to: string; from: string; name: string }) {
  return (
    <Button size="sm" asChild>
      <Link
        to={to}
        state={{ from, autoplay: true }}
        aria-label={t.groups.detail.playFor(name)}
      >
        <Play aria-hidden="true" />
        {t.groups.detail.play}
      </Link>
    </Button>
  );
}

/**
 * GoneState は開いたものがもう無いとき（404）の本体である。「Back to library」でライブラリへ
 * 戻る。
 */
export function GoneState({
  title,
  description,
}: {
  title: UiText;
  description: UiText;
}) {
  return (
    <EmptyState
      icon={<FolderX aria-hidden="true" />}
      title={title}
      description={description}
      action={
        <Button size="sm" asChild>
          <Link to={LIBRARY_PATH}>{t.groups.detail.gone.action}</Link>
        </Button>
      }
    />
  );
}
