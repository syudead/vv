import type { FormEvent } from "react";

import { t } from "@/i18n";
import { CenteredForm } from "@/ui/patterns/centered-form";
import { Button } from "@/ui/shadcn/button";
import { Field, FieldLabel } from "@/ui/shadcn/field";
import { Input } from "@/ui/shadcn/input";

// registry:block centered-form-example. See web/registry/rules/patterns.md, Centered form.

export function CenteredFormExample() {
  const p = t.designSystem.pattern;
  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
  };
  return (
    <CenteredForm
      title={p.signIn}
      description={p.signInDescription}
      onSubmit={submit}
      submit={<Button type="submit">{p.signIn}</Button>}
      footer={p.signInFooter}
    >
      <Field>
        <FieldLabel htmlFor="centered-form-example-user">{p.userName}</FieldLabel>
        <Input id="centered-form-example-user" autoComplete="username" />
      </Field>
      <Field>
        <FieldLabel htmlFor="centered-form-example-password">{p.password}</FieldLabel>
        <Input
          id="centered-form-example-password"
          type="password"
          autoComplete="current-password"
        />
      </Field>
    </CenteredForm>
  );
}
