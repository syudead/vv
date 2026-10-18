import { ArrowLeft, ImageIcon } from "lucide-react";

import { t } from "@/i18n";
import { FormPage } from "@/ui/patterns/form-page";
import { Button } from "@/ui/shadcn/button";
import { Field, FieldDescription, FieldLabel } from "@/ui/shadcn/field";
import { Input } from "@/ui/shadcn/input";
import { Textarea } from "@/ui/shadcn/textarea";

// registry:block form-page-example. See web/registry/rules/patterns.md, Form page.

export function FormPageExample() {
  const p = t.designSystem.pattern;
  return (
    <FormPage
      title={p.formPageTitle}
      detail={p.videoTitles.harbour}
      actions={
        <>
          <Button variant="ghost" size="sm">
            {p.cancel}
          </Button>
          <Button size="sm">{p.save}</Button>
        </>
      }
      asideLabel={p.formPageAside}
      aside={
        <>
          <div className="flex aspect-video w-full items-center justify-center rounded-md bg-card text-muted-foreground">
            <ImageIcon className="size-6" aria-hidden="true" />
          </div>
          <p className="font-medium wrap-anywhere">{p.formPageFile}</p>
          <p className="text-muted-foreground tabular-nums">{p.formPageFacts}</p>
          <Button
            asChild
            variant="link"
            size="sm"
            className="self-start px-0 text-muted-foreground hover:text-primary"
          >
            <a href="#form-page">
              <ArrowLeft aria-hidden="true" />
              {p.back}
            </a>
          </Button>
        </>
      }
      panels={[
        {
          key: "basics",
          legend: p.formPageBasics,
          content: (
            <>
              <Field>
                <FieldLabel htmlFor="form-page-example-name">{p.formPageName}</FieldLabel>
                <Input id="form-page-example-name" placeholder={p.videoTitles.harbour} />
              </Field>
              <Field>
                <FieldLabel htmlFor="form-page-example-notes">
                  {p.formPageNotes}
                </FieldLabel>
                <Textarea id="form-page-example-notes" rows={4} />
                <FieldDescription>{p.formPageNotesHint}</FieldDescription>
              </Field>
            </>
          ),
        },
        {
          key: "more",
          legend: p.formPageMore,
          content: (
            <Field>
              <FieldLabel htmlFor="form-page-example-code">{p.formPageCode}</FieldLabel>
              <Input id="form-page-example-code" />
            </Field>
          ),
        },
      ]}
    />
  );
}
