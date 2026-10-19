import {
  ArrowLeft,
  CircleAlert,
  CircleCheck,
  Minus,
  RotateCw,
  type LucideIcon,
} from "lucide-react";
import type { MouseEvent } from "react";

import { t } from "@/i18n";
import { cn } from "@/lib/cn";
import { FactList } from "@/ui/patterns/fact-list";
import { PageSection } from "@/ui/patterns/page-section";
import { RecordPage } from "@/ui/patterns/record-page";
import { Alert, AlertAction, AlertDescription, AlertTitle } from "@/ui/shadcn/alert";
import { Button } from "@/ui/shadcn/button";

// registry:block record-page-example. Real pages use router links for the back link and Open.

const stages: {
  id: "probe" | "thumbnail" | "preview";
  icon: LucideIcon;
  className: string;
  state: "done" | "failed" | "absent";
}[] = [
  { id: "probe", icon: CircleCheck, className: "text-success", state: "done" },
  { id: "thumbnail", icon: CircleAlert, className: "text-destructive", state: "failed" },
  { id: "preview", icon: Minus, className: "text-muted-foreground", state: "absent" },
];

const stay = (event: MouseEvent) => event.preventDefault();

export function RecordPageExample() {
  const a = t.activity;
  const p = t.designSystem.pattern;
  return (
    <RecordPage
      back={
        <Button asChild variant="ghost" size="sm">
          <a href="#record-example" onClick={stay}>
            <ArrowLeft aria-hidden="true" />
            {a.title}
          </a>
        </Button>
      }
      title={p.recordTitle}
      description={a.kind.import}
      actions={
        <Button asChild variant="outline" size="sm">
          <a href="#record-example" onClick={stay}>
            {a.action.open}
          </a>
        </Button>
      }
      notice={
        <Alert variant="destructive">
          <CircleAlert aria-hidden="true" />
          <AlertTitle>{a.result.importFailed(a.stage.thumbnail, "")}</AlertTitle>
          <AlertDescription>
            <p className="whitespace-pre-line wrap-anywhere">{p.recordMessage}</p>
          </AlertDescription>
          <AlertAction>
            <Button variant="outline" size="sm">
              <RotateCw aria-hidden="true" />
              {a.action.retry}
            </Button>
          </AlertAction>
        </Alert>
      }
    >
      <PageSection title={a.detail.section.summary}>
        <FactList
          facts={[
            { id: "status", term: a.detail.fact.status, value: a.statusValue.attention },
            { id: "process", term: a.detail.fact.process, value: a.kind.import },
            { id: "decider", term: a.detail.fact.decider, value: a.deciderValue.rule },
          ]}
        />
      </PageSection>
      <PageSection title={a.detail.section.stages}>
        {stages.map(({ id, icon: Icon, className, state }) => (
          <div key={id} className="flex items-start gap-3">
            <span className="flex h-5 shrink-0 items-center">
              <Icon aria-hidden="true" className={cn("size-4", className)} />
            </span>
            <div className="flex min-w-0 flex-1 flex-col gap-1">
              <span>{a.stage[id]}</span>
              <span className="text-muted-foreground">{a.detail.stageState[state]}</span>
              {state === "failed" && (
                <p className="whitespace-pre-line wrap-anywhere">{p.recordMessage}</p>
              )}
            </div>
          </div>
        ))}
      </PageSection>
    </RecordPage>
  );
}
