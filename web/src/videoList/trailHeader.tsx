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

/** Show only Library when content is unavailable; do not expose inaccessible names. */
export function UnavailableTrailHeader({ label }: { label: UiText }) {
  return (
    <>
      <Breadcrumbs label={label} crumbs={[{ label: t.home.title }]} keepFirst />
      <PageHeader title={t.home.title} />
    </>
  );
}

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
