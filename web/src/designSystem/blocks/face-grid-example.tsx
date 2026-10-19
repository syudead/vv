import { t } from "@/i18n";
import { CardGrid } from "@/ui/patterns/card-grid";
import { LoadingState } from "@/ui/patterns/loading-state";
import { PersonFace } from "@/ui/PersonFace";

// registry:block face-grid-example. See web/registry/rules/patterns.md, Face grid.

export function FaceGridExample({ loading = false }: { loading?: boolean }) {
  const p = t.designSystem.pattern;
  if (loading) {
    return <LoadingState label={p.faceGridLoading} layout="grid" size="face" count={4} />;
  }
  return (
    <CardGrid size="face">
      {p.faceGridPeople.map((person) => (
        <a
          key={person.name}
          href="#people-grid"
          className="flex min-w-0 flex-col items-center gap-2 rounded-md p-1 transition-shadow hover:shadow-card-hover motion-reduce:transition-none"
        >
          <PersonFace name={person.name} shape="card" />
          <span className="flex w-full min-w-0 flex-col items-center text-center">
            <span title={person.name} className="w-full truncate text-sm font-medium">
              {person.name}
            </span>
            <span className="w-full truncate text-xs text-muted-foreground">
              {person.meta}
            </span>
          </span>
        </a>
      ))}
    </CardGrid>
  );
}
