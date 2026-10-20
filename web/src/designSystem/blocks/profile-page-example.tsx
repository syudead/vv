import { t } from "@/i18n";
import { FactList } from "@/ui/patterns/fact-list";
import { ProfilePage } from "@/ui/patterns/profile-page";
import { Shelf, ShelfItem } from "@/ui/patterns/shelf";
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
          <div className="flex flex-col gap-1">
            <h1 className="line-clamp-3 text-xl font-semibold">{p.profilePageName}</h1>
            <p className="text-sm text-muted-foreground">{p.profilePageAliases}</p>
          </div>
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
          <div className="flex flex-wrap items-center gap-2">
            <Button variant="outline" size="sm">
              {p.profilePageEdit}
            </Button>
          </div>
        </>
      }
      facts={
        <FactList
          facts={[{ id: "born", term: p.profilePageBorn, value: p.profilePageBornValue }]}
        />
      }
    >
      <Shelf
        title={p.profilePagePerformer}
        count={p.profilePageCount}
        action={
          <Button variant="ghost" size="sm">
            {p.profilePageSeeAll}
          </Button>
        }
      >
        {titles.map((title) => (
          <ShelfItem key={title}>
            <div className="flex flex-col gap-2">
              <span className="aspect-video w-full rounded-md bg-card" />
              <span className="truncate text-sm font-medium">{title}</span>
            </div>
          </ShelfItem>
        ))}
      </Shelf>
    </ProfilePage>
  );
}
