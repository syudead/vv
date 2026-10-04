# Contract: Host command for resetting the account

Parent Issue: #135.

After first-time setup, the server administrator changes the username and
resets the password only from the host shell (requirement 3). The first account
is created in the first-time setup screen
([auth-api.md, `POST /api/auth/setup`](auth-api.md#post-apiauthsetup)). The commands are added as
subcommands of the existing `mdm` binary; `mdm` with no arguments still starts
the server. Settings are read from the same `MDM_DATA_DIR` as server startup
(Runtime settings in
[docs/how-to/running-vv.md](../../../docs/how-to/running-vv.md)).

## Subcommands

| Command | Action |
| --- | --- |
| `mdm account set-username <NAME>` | Change the username |
| `mdm account set-password` | Reset the password |

- Both write only when the account is configured. When it is not, they write
  nothing to `account` or `sessions`, tell the user on standard error to run
  first-time setup in the browser, and exit with code 2. Creating the account
  from the command would give two ways to create it, alongside first-time setup
  in the browser, and an intermediate state where only one is configured.
- Both invalidate every existing session in the same transaction as the write
  ([data-model.md, Write rules](../data-model.md#write-rules)). They run whether the
  server is running or stopped. A running server rejects old sessions from the
  next request on and cuts off long responses in progress.
- Only `MDM_DATA_DIR` is needed; the server's startup checks (presence of
  `ffmpeg` and `ffprobe`) do not run, so the password can be reset on a host
  without `ffmpeg`.
- Migrations are applied before writing, as at startup.
- Under Docker Compose, call `docker compose exec mdm mdm account set-password`.
  When the container is stopped, use
  `docker compose run --rm mdm account set-password`.

## Reading the password

| Standard input | Behaviour |
| --- | --- |
| A terminal | Echo is turned off and the password is asked twice. On mismatch the command exits without writing to `account` or `sessions` |
| Not a terminal | The first line (without the trailing newline) is the password, so scripts and `docker compose exec -T` can pass it |

- The password is never accepted as an argument or an environment variable: the
  plain text would remain in shell history, `ps` and `docker inspect`.
- Values follow
  [data-model.md, Username and password values](../data-model.md#username-and-password-values). A value
  that breaks the rules writes nothing to `account` or `sessions`, and the
  reason goes to standard error.

## Exit codes and output

| Result | Exit code | Standard error |
| --- | --- | --- |
| Written | 0 | What changed, and that existing sessions were invalidated |
| Not configured, invalid value, confirmation mismatch, unknown subcommand | 2 | The reason and usage |
| The DB cannot be opened or written | 1 | The reason |

The password and its hash are never printed, whatever the result.
