# Quickstart: A real proxy and a real site, from the command line and the server

These steps cover what the automated checks cannot: CI's plugins answer
without the internet, its proxy and address tests run against loopback, and
CI does not run the Windows app. Setup and checks:
[development.md](../../docs/how-to/development.md),
[running-vv.md](../../docs/how-to/running-vv.md).

## Prerequisites

- A plugin built from the guide's steps that fetches a public site, with a
  setting that declares `options` and a `default`, a `secret` setting, and a
  variant whose
  `fetch` requests the LAN address of the tester's router.
- An HTTP proxy on the LAN that the tester controls (one that logs requests),
  and its address with a wrong password for the authentication step.
- vv running from the container image, and `VVMDM.exe` on Windows, each with
  one imported video.

## Steps

| Step | Expected result | Acceptance |
| --- | --- | --- |
| `mdm plugin pack` with a manifest whose default is not among its options | The reason names `default` and `options`; the module's bytes are unchanged | 3, edge cases |
| `mdm plugin pack` with the corrected manifest, then `mdm plugin run plugin.wasm fetch <work ID>` | The kept `workInfo` is printed; a line the plugin wrote to standard error appears on the command's standard error | 1, 4 |
| `mdm plugin run` with the LAN variant | The command exits 1 with `blocked_address` and the router's address | 2 |
| `mdm plugin run` with `--proxy` pointing at the LAN proxy | The proxy's log shows the request; the result is the same as without the proxy | 7 |
| Add the packed plugin on the settings screen, open its settings dialog | The default is shown as the default, and the setting with options is a choice among them | 5 |
| Approve the plugin, enter a value for the secret setting, and fetch without changing the setting with options, with the plugin writing its received settings to standard error | The settings screen's last-run output shows the default value and the secret as `[secret]` | 6, 10, 11 |
| Set the proxy in the Network section and fetch again | The proxy's log shows the request; the fetch succeeds. Clear the proxy and fetch: the proxy's log shows nothing new | 7 |
| With the proxy set, fetch with the LAN variant | The fetch fails with the router's address; the proxy's log shows no request to it | 8 |
| Set the proxy with a wrong password and fetch | The fetch fails as a network error naming the proxy; another plugin without network use still runs | edge cases |
| On the comparison, clear two of the fetched tags and apply | Only the remaining tags are attached as tentative | 9 |
| Repeat the proxy steps in `VVMDM.exe` | The Network section shows the LAN switch and the proxy row; the same results | 7, 8 |
