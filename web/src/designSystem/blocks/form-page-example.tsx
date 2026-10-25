import { ArrowLeft, ImageIcon } from "lucide-react";
import { useState } from "react";

import { t } from "@/i18n";
import { FormPage, FormPageDialog } from "@/ui/patterns/form-page";
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

const dialogNames = ["harbour", "ridge", "market"] as const;

/**
 * The dialog form: the aside holds an input that is not saved itself but derives the panel's
 * values, and the count of its result; the sole panel spans both panel columns.
 */
export function FormPageDialogExample({
  defaultOpen = false,
}: {
  defaultOpen?: boolean;
}) {
  const p = t.designSystem.pattern;
  const [open, setOpen] = useState(defaultOpen);
  const [prefix, setPrefix] = useState("");
  const [typed, setTyped] = useState<ReadonlyMap<string, string>>(new Map());
  const value = (name: (typeof dialogNames)[number]) =>
    typed.get(name) ?? `${prefix}${p.videoTitles[name]}`;
  const renamed = dialogNames.filter((name) => value(name) !== p.videoTitles[name]);
  return (
    <>
      <Button size="sm" onClick={() => setOpen(true)}>
        {p.formPageDialogOpen}
      </Button>
      <FormPageDialog
        open={open}
        onClose={() => setOpen(false)}
        title={p.formPageDialogTitle}
        actions={
          <>
            <Button variant="ghost" size="sm" onClick={() => setOpen(false)}>
              {p.cancel}
            </Button>
            <Button size="sm" onClick={() => setOpen(false)}>
              {p.save}
            </Button>
          </>
        }
        asideLabel={p.formPageDialogAside}
        aside={
          <>
            <Field>
              <FieldLabel htmlFor="form-page-dialog-example-prefix">
                {p.formPageDialogPrefix}
              </FieldLabel>
              <Input
                id="form-page-dialog-example-prefix"
                className="h-8"
                value={prefix}
                onChange={(event) => setPrefix(event.target.value)}
              />
              <FieldDescription>{p.formPageDialogPrefixHint}</FieldDescription>
            </Field>
            <p className="tabular-nums">
              {p.formPageDialogCount(renamed.length, dialogNames.length)}
            </p>
          </>
        }
        panels={[
          {
            key: "names",
            legend: p.formPageDialogNames,
            content: dialogNames.map((name) => (
              <Field key={name}>
                <FieldLabel htmlFor={`form-page-dialog-example-${name}`}>
                  {p.videoTitles[name]}
                </FieldLabel>
                <Input
                  id={`form-page-dialog-example-${name}`}
                  className="h-8"
                  value={value(name)}
                  onChange={(event) =>
                    setTyped((current) => new Map(current).set(name, event.target.value))
                  }
                />
              </Field>
            )),
          },
        ]}
      />
    </>
  );
}
