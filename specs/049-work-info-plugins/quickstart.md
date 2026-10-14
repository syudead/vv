# Quickstart: A real plugin against a real site, on the container and the Windows app

These steps cover what the automated checks cannot: CI's plugins answer
without the internet and its address tests run against loopback, and CI does
not run the Windows app. Setup and checks: [development.md](../../docs/how-to/development.md),
[running-vv.md](../../docs/how-to/running-vv.md).

## Prerequisites

- A plugin built from the
  [plugin contract](contracts/plugin-contract.md) that searches and fetches a
  public site or database API of the tester's choice, with an API key as a
  secret setting when the API needs one, and a variant whose `fetch` requests
  the LAN address of the tester's router.
- vv running from the container image, and `VVMDM.exe` on Windows, each with
  one imported video.

## Steps

| Step | Expected result | Acceptance |
| --- | --- | --- |
| Add the plugin on the settings screen and approve it | It shows as unapproved with its name and version, the network notice and the settings it receives, then as approved | 1 |
| Enter the site's real API key as the plugin's secret setting and reload the page | The setting shows as set, its value is not shown, and the plugin stays approved | 8 |
| On the video page, search with a word from a real work and pick a candidate | Candidates from the site appear; picking one shows the current and fetched title and tags side by side | 3 |
| Select only the title and the tags and apply | The display name changes and the tags appear as tentative; nothing else on the page changes | 4 |
| Fetch with the LAN variant | The fetch fails, the failure names the router's address, and the video is unchanged | 5 |
| Upload a rebuilt plugin with the same name | It returns to unapproved and disappears from the video page's choices | 2 |
| Repeat the first three steps in `VVMDM.exe` | The same results | 1, 3, 8 |
