import { t } from "@/i18n";
import { CardGrid } from "@/ui/patterns/card-grid";
import { LoadingState } from "@/ui/patterns/loading-state";
import { OrganizationLogo } from "@/ui/OrganizationLogo";

// registry:block logo-grid-example. See web/registry/rules/patterns.md, Logo grid.

export function LogoGridExample({ loading = false }: { loading?: boolean }) {
  const p = t.designSystem.pattern;
  if (loading) {
    return <LoadingState label={p.logoGridLoading} layout="grid" size="logo" count={3} />;
  }
  return (
    <CardGrid size="logo">
      {p.logoGridOrganizations.map((organization) => (
        <a
          key={organization.name}
          href="#logo-grid"
          className="flex min-w-0 flex-col gap-2 rounded-md p-1 transition-shadow hover:shadow-card-hover motion-reduce:transition-none"
        >
          <OrganizationLogo
            alt={organization.name}
            name={organization.name}
            shape="card"
          />
          <span className="flex w-full min-w-0 flex-col">
            <span
              title={organization.name}
              className="w-full truncate text-sm font-medium"
            >
              {organization.name}
            </span>
            <span className="w-full truncate text-xs text-muted-foreground tabular-nums">
              {organization.meta}
            </span>
          </span>
        </a>
      ))}
    </CardGrid>
  );
}
