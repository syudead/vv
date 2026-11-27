import { t } from "@/i18n";
import { FactList } from "@/ui/patterns/fact-list";
import { ProfilePage } from "@/ui/patterns/profile-page";
import { Shelf } from "@/ui/patterns/shelf";
import { OrganizationLogo } from "@/ui/OrganizationLogo";
import { Button } from "@/ui/shadcn/button";

// registry:block organization-page-example. See web/registry/rules/patterns.md, Profile page.

export function OrganizationPageExample() {
  const p = t.designSystem.pattern;
  return (
    <ProfilePage
      profileLabel={p.organizationPageLabel}
      face={<OrganizationLogo alt={p.organizationPageName} shape="profile" />}
      identity={
        <>
          <div className="flex flex-col gap-1">
            <h1 className="line-clamp-3 text-xl font-semibold">
              {p.organizationPageName}
            </h1>
            <p className="text-sm text-muted-foreground">{p.organizationPageAliases}</p>
          </div>
          <div className="flex flex-wrap items-center gap-2">
            <Button variant="secondary" size="sm">
              {p.profilePageEdit}
            </Button>
          </div>
        </>
      }
      facts={
        <FactList
          facts={[
            {
              id: "founded",
              term: p.organizationPageFounded,
              value: (
                <span className="tabular-nums">{p.organizationPageFoundedValue}</span>
              ),
            },
            {
              id: "location",
              term: p.organizationPageLocation,
              value: p.organizationPageLocationValue,
            },
          ]}
        />
      }
    >
      <Shelf title={p.organizationPageFamily}>
        <FactList
          facts={[
            {
              id: "parent",
              term: p.organizationPageParent,
              value: p.organizationPageParentValue,
            },
            {
              id: "children",
              term: p.organizationPageChildren,
              value: p.organizationPageChildrenValue,
            },
          ]}
        />
      </Shelf>
    </ProfilePage>
  );
}
