import { Pencil } from "lucide-react";

import { t } from "@/i18n";
import { CardGrid } from "@/ui/patterns/card-grid";
import { PageHeader } from "@/ui/patterns/page-header";
import { ProfilePage } from "@/ui/patterns/profile-page";
import { PersonFace } from "@/ui/PersonFace";
import { Button } from "@/ui/shadcn/button";

// registry:block profile-page-example. See web/registry/rules/patterns.md, Profile page.

export function ProfilePageExample() {
  const p = t.designSystem.pattern;
  const titles = [
    p.videoTitles.harbour,
    p.videoTitles.eikando,
    p.videoTitles.train,
    p.videoTitles.lighthouse,
  ];
  return (
    <ProfilePage
      profileLabel={p.profilePageLabel}
      face={<PersonFace name={p.profilePageName} shape="profile" />}
      identity={
        <>
          <h1 className="line-clamp-3 text-xl font-semibold">{p.profilePageName}</h1>
          <p className="text-sm text-muted-foreground">{p.profilePageAliases}</p>
        </>
      }
      actions={
        <Button variant="secondary" size="sm">
          <Pencil aria-hidden="true" />
          {p.profilePageEdit}
        </Button>
      }
      details={
        <>
          <dl className="flex flex-wrap gap-x-8 gap-y-3">
            {[
              { term: p.profilePageAge, value: p.profilePageAgeValue },
              { term: p.profilePageHeight, value: p.profilePageHeightValue },
            ].map((figure) => (
              <div key={figure.term} className="flex flex-col-reverse gap-0.5">
                <dt className="text-xs text-muted-foreground">{figure.term}</dt>
                <dd className="text-lg font-semibold tabular-nums">{figure.value}</dd>
              </div>
            ))}
          </dl>
          <dl className="grid grid-cols-2 gap-x-6 gap-y-3 text-sm md:grid-cols-3 xl:grid-cols-4">
            <div className="flex min-w-0 flex-col-reverse gap-0.5">
              <dt className="text-xs text-muted-foreground">{p.profilePageBorn}</dt>
              <dd className="wrap-anywhere">{p.profilePageBornValue}</dd>
            </div>
          </dl>
        </>
      }
    >
      <section
        aria-label={p.profilePagePerformer}
        className="flex min-w-0 flex-col gap-3"
      >
        <PageHeader
          title={p.profilePagePerformer}
          headingLevel={2}
          count={p.profilePageCount}
          actions={
            <Button variant="ghost" size="sm">
              {p.profilePageSeeAll}
            </Button>
          }
        />
        <CardGrid size={0}>
          {titles.map((title) => (
            <div key={title} className="flex flex-col gap-2">
              <span className="aspect-video w-full rounded-md bg-card" />
              <span className="truncate text-sm font-medium">{title}</span>
            </div>
          ))}
        </CardGrid>
      </section>
    </ProfilePage>
  );
}
