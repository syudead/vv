import { t } from "@/i18n";
import { StepsPage } from "@/ui/patterns/steps-page";
import { Button } from "@/ui/shadcn/button";
import { ToggleGroup, ToggleGroupItem } from "@/ui/shadcn/toggle-group";

// 手順の画面の見本（registry:block steps-page-example）。3 つの手順の 2 つ目を開いた形で、
// 中身に分野の切り替えを並べ、下端の左に Skip（ghost の sm）、右に 1 つの主操作（default の sm）を
// 置く。写した画面は手順・題・中身・操作を自分のものに差し替え、間隔と幅は書かない
// （web/registry/rules/patterns.md の Steps page）。

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
        className="w-full flex-wrap gap-2"
      >
        {topics.map(([key, name]) => (
          <ToggleGroupItem
            key={key}
            value={key}
            className="flex-none rounded-md px-3 data-[variant=outline]:border-l"
          >
            {name}
          </ToggleGroupItem>
        ))}
      </ToggleGroup>
    </StepsPage>
  );
}
