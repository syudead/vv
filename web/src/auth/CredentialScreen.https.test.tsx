/**
 * @vitest-environment-options { "url": "https://vv.example/login" }
 */
import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import { describe, expect, it } from "vitest";

import LoginPage from "./LoginPage";
import SetupPage from "./SetupPage";

// HTTPS では警告の行そのものを出さず、肯定の文も出さない
// （specs/016-single-account-auth/ui-design.md「Connection warning」）。
describe("HTTPS の接続", () => {
  it("ページが https: なら前提を確かめる", () => {
    expect(window.location.protocol).toBe("https:");
  });

  it("ログイン画面に警告を出さない", () => {
    render(
      <MemoryRouter initialEntries={["/login"]}>
        <LoginPage />
      </MemoryRouter>,
    );
    expect(screen.queryByText(/暗号化/)).toBeNull();
    expect(screen.queryByText(/安全/)).toBeNull();
    expect(document.getElementById("connection-warning")).toBeNull();
    expect(
      screen.getByLabelText("ユーザー名").getAttribute("aria-describedby"),
    ).toBeNull();
    expect(
      screen.getByRole("button", { name: "ログイン" }).getAttribute("aria-describedby"),
    ).toBeNull();
  });

  it("初回設定画面に警告を出さない", () => {
    render(<SetupPage />);
    expect(screen.queryByText(/暗号化/)).toBeNull();
    expect(screen.queryByText(/安全/)).toBeNull();
    expect(document.getElementById("connection-warning")).toBeNull();
  });
});
