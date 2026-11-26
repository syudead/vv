import { act, render, screen } from "@testing-library/react";
import {
  MemoryRouter,
  useLocation,
  useNavigate,
  type NavigateFunction,
} from "react-router";
import { describe, expect, it } from "vitest";

import { useTagListCriteria } from "./tagListUrl";

type Apply = ReturnType<typeof useTagListCriteria>["apply"];

let apply: Apply;
let navigate: NavigateFunction;

function Probe() {
  apply = useTagListCriteria("name").apply;
  navigate = useNavigate();
  return <output>{useLocation().search}</output>;
}

describe("useTagListCriteria apply", () => {
  it("描き直す前に同じ条件を続けて積んでも、履歴の項目は1つだけ増える", async () => {
    render(
      <MemoryRouter initialEntries={["/tags?sort=name"]}>
        <Probe />
      </MemoryRouter>,
    );

    await act(async () => {
      apply({ unusedOnly: true }, "push");
      apply({ unusedOnly: true }, "push");
    });
    expect(screen.getByRole("status").textContent).toBe("?unused=1&sort=name");

    await act(async () => {
      void navigate(-1);
    });
    expect(screen.getByRole("status").textContent).toBe("?sort=name");
  });
});
