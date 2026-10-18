import { act, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, it, vi } from "vitest";

import { t } from "../i18n";

import {
  json,
  renderPage,
  reveal,
  seed,
  server,
  setUpGroupPageServer,
} from "../testing/groupPage";

setUpGroupPageServer();

it("検索条件を初回・次ページに渡し、検索の変更と解除をURLと一覧へ反映する", async () => {
  seed();
  vi.stubGlobal("scrollTo", vi.fn());
  const original = fetch;
  const requests: URL[] = [];
  const stored = server.groups.get(1)!;
  vi.stubGlobal(
    "fetch",
    vi.fn<typeof fetch>((input, init) => {
      const url = new URL(String(input), "http://localhost");
      const query = url.searchParams.get("query");
      if (url.pathname.startsWith("/api/groups/1") && query !== null) {
        requests.push(url);
        if (url.pathname.endsWith("/items")) {
          return Promise.resolve(json({ items: [stored.items[2]] }));
        }
        return Promise.resolve(
          json({
            group: stored.group,
            trail: [],
            items: query === "Episode" ? [stored.items[0]] : [],
            nextCursor: query === "Episode" ? "next-match" : undefined,
          }),
        );
      }
      return original(input, init);
    }),
  );
  const user = userEvent.setup();
  renderPage("/groups/1?q=Episode");
  await screen.findByRole("link", { name: "Episode 1" });
  expect(screen.queryByRole("link", { name: "Episode 2" })).toBeNull();
  expect(requests[0]?.searchParams.get("query")).toBe("Episode");

  act(reveal);
  await screen.findByRole("link", { name: "Episode 2" });
  const more = requests.find((url) => url.pathname.endsWith("/items"));
  expect(more?.searchParams.get("query")).toBe("Episode");
  expect(more?.searchParams.get("cursor")).toBe("next-match");

  const search = screen.getByRole("searchbox", { name: "Search videos" });
  await user.clear(search);
  await user.type(search, "missing");
  await user.tab();
  await waitFor(() => expect(requests.at(-1)?.searchParams.get("query")).toBe("missing"));
  await screen.findByRole("heading", { name: t.list.noMatches });
  expect(screen.queryByRole("link", { name: "Episode 1" })).toBeNull();
  expect(screen.getByRole("link", { name: 'Play "Season"' }).getAttribute("href")).toBe(
    "/videos/10?group=1",
  );

  await user.click(screen.getByRole("button", { name: "Clear search" }));
  await screen.findByRole("link", { name: "Extras, group of 1 video" });
  expect(screen.getByRole("searchbox", { name: "Search videos" })).toHaveProperty(
    "value",
    "",
  );
  await user.click(screen.getByRole("link", { name: "Episode 1" }));
  await screen.findByText('Opened /videos/10?group=1 {"from":"/groups/1"}');
});
