# Contract: host commands that reset the account

Parent Issue: #135.

After the first setup, the server administrator changes the username and resets the password
from the host shell, and only from there (Requirement 3). The first account is created by the
first setup in the UI ([auth-api.md §2](auth-api.md#2-post-apiauthsetup)).

- The commands are subcommands of the existing `mdm` binary. `mdm` with no arguments still
  starts the server.
- Settings come from the same `MDM_DATA_DIR` as the server start
  ([docs/how-to/running-vv.md](../../../docs/how-to/running-vv.md), Runtime settings).

## 1. Subcommands

| Command | Effect |
| --- | --- |
| `mdm account set-username <NAME>` | Changes the username |
| `mdm account set-password` | Resets the password |

- Both commands write only when the account is configured. When it is not configured, they
  write nothing to `account` or `sessions`, print to stderr that the first setup must be done
  in the UI, and exit with code 2. Reason: if a command could create the account, there would
  be two ways to create it, and an intermediate state configured by only one of them.
- Both commands invalidate every existing session in the same transaction as the write
  ([data-model.md §5](../data-model.md#5-write-rules)). They run whether the server is
  running or stopped. A running server rejects old sessions from the next request and cuts
  off long responses in progress.
- They need only `MDM_DATA_DIR`. They skip the server's startup checks (presence of `ffmpeg`
  and `ffprobe`), so the password can be reset on a host without `ffmpeg`.
- They apply migrations before writing, as the server start does.
- With Docker Compose, call `docker compose exec mdm mdm account set-password`. When the
  container is stopped, use `docker compose run --rm mdm account set-password`.

## 2. Reading the password

- When stdin is a terminal, the command disables echo and asks twice. If the two entries
  differ, it exits without writing to `account` or `sessions`.
- When stdin is not a terminal, the first line (without the trailing newline) is the
  password. This supports scripts and `docker compose exec -T`.
- The password is never taken from an argument or an environment variable, because the
  plaintext would remain in the shell history, `ps` and `docker inspect`.
- The value follows [data-model.md §6](../data-model.md#6-username-and-password-values). A value
  outside the rules writes nothing to `account` or `sessions`, and the reason goes to stderr.

## 3. Exit codes and output

| Result | Exit code | stderr |
| --- | --- | --- |
| Written | 0 | What changed, and that existing sessions were invalidated |
| Not configured, invalid value, confirmation mismatch, unknown subcommand | 2 | Reason and usage |
| The DB cannot be opened or written | 1 | Reason |

No result prints the password or its hash.
