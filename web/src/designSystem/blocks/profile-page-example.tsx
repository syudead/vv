import { t } from "@/i18n";
import { CardGrid } from "@/ui/patterns/card-grid";
import { FactList } from "@/ui/patterns/fact-list";
import { PageHeader } from "@/ui/patterns/page-header";
import { ProfilePage } from "@/ui/patterns/profile-page";
import { PersonFace } from "@/ui/PersonFace";
import { Button } from "@/ui/shadcn/button";
import { ToggleGroup, ToggleGroupItem } from "@/ui/shadcn/toggle-group";

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
          <div className="flex flex-wrap items-center gap-2">
            <Button variant="outline" size="sm">
              {p.profilePageEdit}
            </Button>
          </div>
        </>
      }
      details={
        <section className="flex flex-col gap-2 text-sm">
          <h2 className="text-sm font-semibold">{p.profilePageGroup}</h2>
          <FactList
            facts={[
              { id: "born", term: p.profilePageBorn, value: p.profilePageBornValue },
              {
                id: "height",
                term: p.profilePageHeight,
                value: p.profilePageHeightValue,
              },
            ]}
          />
        </section>
      }
      header={
        <PageHeader
          title={p.profilePageVideos}
          headingLevel={2}
          count={p.profilePageCount}
        />
      }
      toolbar={
        <ToggleGroup
          type="single"
          variant="outline"
          size="sm"
          aria-label={p.profilePageRoles}
          defaultValue="all"
        >
          <ToggleGroupItem value="all" className="flex-none px-3">
            {p.profilePageAll}
          </ToggleGroupItem>
          <ToggleGroupItem value="director" className="flex-none px-3">
            {p.profilePageDirector}
          </ToggleGroupItem>
        </ToggleGroup>
      }
    >
      <CardGrid size={0}>
        {titles.map((title) => (
          <div key={title} className="flex flex-col gap-2">
            <span className="aspect-video w-full rounded-md bg-card" />
            <span className="truncate text-sm font-medium">{title}</span>
          </div>
        ))}
      </CardGrid>
    </ProfilePage>
  );
}
