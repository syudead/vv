# Tech stack selection: MDM (Media Data Management)

- Scope: the technology choices for a system that manages video files and plays them in a browser

## 1. Premises

These are the current reasons for the choices. The current features and data boundaries are in
[`ARCHITECTURE.md`](../../ARCHITECTURE.md).

| Item | Decision |
| --- | --- |
| Deployment | Self-hosted (one personal or home machine, Docker on a NAS or small server) |
| Client | Web browser only (PC and smartphone) |
| Video handling | Supported formats are served from the original file; unsupported formats are live-transcoded during the request |
| Users | A single account. Guests can view public videos |
| Library size | Up to tens of thousands of videos and several TB on local disks |

## 2. Selection criteria

For a self-hosted single-user system, **operational simplicity** and **recoverability after
breakage** come before throughput.

1. **Run as one process in one container.** Add no always-on external middleware (Redis, a message
   broker, a separate DB server).
2. **Separate the rebuildable index from user data.** A scan can rebuild the index from the video
   files, but not playback positions, credentials and similar data
   ([Data classes](../../ARCHITECTURE.md#rebuildable-and-user-data)).
3. **Enforce boundaries mechanically.** Following the principles in `ARCHITECTURE.md`, lint
   enforces a structure in which the domain layer does not depend on HTTP, the DB or ffmpeg.
4. **Keep heavy work inside a boundary.** Work that uses `ffmpeg` stays in an adapter and is not
   mixed with HTTP or storage responsibilities.

## 3. Decisions

| Layer | Choice | Main reason |
| --- | --- | --- |
| Backend language | Go (version in `go.mod`) | Ships as a single binary; the standard library covers a resident process and child process management. Low memory use on a NAS |
| HTTP server | Standard library `net/http` (`ServeMux` of Go 1.22 and later) | Method-based routing and path wildcards are built in. `http.ServeContent` already implements Range serving correctly |
| Video delivery | `http.ServeContent` for supported formats, per-request fragmented MP4 live transcoding for unsupported formats | Range serving of the original file is left to the standard implementation, and transcoded output is not stored ([Live transcoding seek](live-transcode-seek.md)) |
| Frontend | React + Vite + React Router + Tailwind CSS | An SPA whose static build is embedded in the Go binary with `embed`. React Router manages navigation |
| API contract | OpenAPI 3.1 as the source of truth, generated with `oapi-codegen` for Go and `openapi-typescript` for TS | Mechanically prevents the one cost a two-language setup adds (type drift) |
| DB | SQLite (`modernc.org/sqlite`, no CGO, WAL mode) | Cross-compiles as a static binary and fits a small alpine-based image |
| Queries | Hand-written SQL with `database/sql` | Keeps SQL, including FTS5, as the primary source |
| Migrations | `goose` (migrations bundled in `embed.FS`) | The binary applies them alone, with no external tool to install |
| Full-text search | SQLite FTS5 (`tokenize='trigram'`) | Substring search of Japanese without an extra tokenizer. No external search engine |
| Media probing | `ffprobe` / `ffmpeg` run with `os/exec` (timeouts via `context`) | No wrapper, so arguments and failure reasons are explicit. The standard library is enough to control process shutdown |
| Authentication | Username and password (Argon2id from `golang.org/x/crypto/argon2`) + HttpOnly cookie sessions (sessions stored in SQLite) | Sufficient for a single user. Public exposure requires an HTTPS reverse proxy |
| Background work | A SQLite job table + goroutine workers (graceful shutdown via `context`) | No separate process or broker. Jobs resume after a restart |
| Logging | Standard library `log/slog` (JSON handler) | Structured logs without extra dependencies |
| Tests | Go standard `testing` + `net/http/httptest` (Range checks) + Playwright (playback E2E) | Only E2E can prove that "playback actually starts" |
| Lint | `golangci-lint` (`depguard` forbids imports across layers) | CI fails mechanically on dependency direction violations |
| Distribution | Docker (multi-stage, alpine + ffmpeg) + Compose | No CGO, so it runs on alpine as is, with a small image |

Dependency direction and package responsibilities are in
[ARCHITECTURE.md](../../ARCHITECTURE.md#intended-dependency-direction). SQLite holds both the
rebuildable index and user data, so the distinction during recovery follows
[the data classes in the same document](../../ARCHITECTURE.md#rebuildable-and-user-data). The
content key, which identifies a video as the same after a move or rename, is built from the first
and last 1 MiB and the file size. It identifies a file without reading all of it, and needs only
the standard library's SHA-256.

## 4. Rejected options

| Candidate | Reason for rejection |
| --- | --- |
| TypeScript / Node.js backend | Range serving and process management would be hand-written, and native dependencies (`better-sqlite3`) would need rebuild maintenance |
| Python + FastAPI | Rich libraries, but heavy distribution, and the operating cost of resident workers and dependency management does not suit self-hosting |
| Echo / Gin / Fiber | The standard `ServeMux` is enough at this scale, so there is no reason to add a dependency for routing. Fiber is not `net/http` compatible and would also lose the benefit of `ServeContent` |
| GORM / ent | Hand-written SQL was chosen over ORM abstraction because it keeps SQL, including FTS5, as the primary source |
| `sqlc` | Not adopted, given the policy of managing SQL, including FTS5, as is |
| `mattn/go-sqlite3` | Needs CGO, which adds cross-compilation and alpine build maintenance |
| PostgreSQL | A single-account deployment should not need a separate container and a separate backup procedure |
| Meilisearch / Elasticsearch | Better search quality, but one more resident process. FTS5 trigram is practical for tens of thousands of items |
| SvelteKit | Light and a strong option. React was chosen only for its ecosystem and future hiring, not for technical superiority |
| Bulk HLS transcoding at startup | Consumes large amounts of storage and CPU; per-request live transcoding is used instead |
| Redis + an external job queue | Probing, thumbnails and previews are processed one at a time per stage. A SQLite job table and goroutines are enough |
| S3 / MinIO | Conflicts with the premise that the local disk is the source of truth, and adds a relay to Range serving |
| Adopting Jellyfin / Plex | Off-the-shelf products meet the requirements, but this repository assumes a custom build. They serve only as references for feature comparison |

## 5. Known risks and mitigations

- **Two languages for frontend and backend.** Type drift and duplicated builds and CI. The API
  treats `api/openapi.yaml` as the single source of truth and generates both Go and TypeScript
  from it, so drift shows up as a compile error. The build is one task in `Taskfile.yml`
  (`task build`: SPA build → embed → Go build).
- **The FTS5 trigram tokenizer.** Creation and search are verified with `modernc.org/sqlite`.
  Trigram `MATCH` does not match search terms of two characters or fewer, so search has two paths:
  `MATCH` for 3 or more characters, and `instr` on the matching key `search_key` for 1–2
  characters (routing in `termUsesMatch` in `internal/store/search.go`, verified in
  `internal/store/fts_test.go`).
- **Unicode normalization of file names.** Real paths keep the spelling the file system returned.
  Only display names and search strings are normalized to NFC (`golang.org/x/text/unicode/norm`).
  Real paths are not normalized, because on Linux and similar systems the result can point to a
  different, nonexistent entry.
- **I/O saturation during large scans.** Probing, thumbnails and previews each have a per-stage
  worker that processes one item at a time (parallelism 1 within a stage). Progress is counted
  from the `jobs` table and sent to the UI over `/api/events`.
