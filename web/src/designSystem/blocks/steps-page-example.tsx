import { t } from "@/i18n";
import { StepsPage } from "@/ui/patterns/steps-page";
import { Button } from "@/ui/shadcn/button";
import { ToggleGroup, ToggleGroupItem } from "@/ui/shadcn/toggle-group";

// The steps page sample (registry:block steps-page-example): the second of three steps, with interest
// toggles as its content, Skip at the bottom left and one main action at the right. A copy replaces the
// steps, title, content and actions and writes no spacing or widths (web/registry/rules/patterns.md, Steps page).

const stepKeys = ["folders", "topics", "finish"] as const;

export function StepsPageExample() {
  const p = t.designSystem.pattern;
  const current = 1;
  const steps = stepKeys.map((key, index) => {
    const name = p.stepNames[key];
    let label: string = name;
    if (index < current) label = p.stepDone(name);
    else if (index === current) label = p.stepCurrent(name);
    return { key, name, label };
  });
  const topics = Object.entries(p.tagNames);
  return (
    <StepsPage
      steps={steps}
      current={current}
      stepsLabel={p.steps}
      progress={p.stepOf(current + 1, steps.length)}
      title={p.stepsTitle}
      description={p.stepsDescription}
      skip={
        <Button variant="ghost" size="sm">
          {p.skip}
        </Button>
      }
      action={<Button size="sm">{p.continue}</Button>}
    >
      <ToggleGroup
        type="multiple"
        variant="outline"
        size="sm"
        aria-label={p.topics}
        defaultValue={["travel"]}
        spacing="separate"
        className="w-full flex-wrap"
      >
        {topics.map(([key, name]) => (
          <ToggleGroupItem key={key} value={key} className="px-3">
            {name}
          </ToggleGroupItem>
        ))}
      </ToggleGroup>
    </StepsPage>
  );
}
