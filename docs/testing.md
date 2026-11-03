# Testing

Tests cover the existing ledger. They use made-up accounts and amounts; they do not require a
Google account. Install the pinned Go and Bun versions with `task setup`. PostgreSQL integration
tests require Docker; browser tests also require Chromium (`cd apps/web && bun x playwright install
--with-deps chromium`).

| Command                 | What it verifies                                                                  | Dependencies                         |
| ----------------------- | --------------------------------------------------------------------------------- | ------------------------------------ |
| `task test:unit`        | Pure Go behavior and TypeScript utilities                                         | Go, Bun                              |
| `task test:integration` | Full Go suite, including PostgreSQL persistence, HTTP API and OAuth/MCP contracts | Go, Docker                           |
| `task e2e`              | Browser workflows against Next.js, Go and PostgreSQL, on desktop and mobile       | Go, Bun, Chromium, migrated database |
| `task test:coverage`    | Go statement coverage with HTML and TypeScript coverage with LCOV                 | Go, Bun, Docker                      |

`task test` continues to run the full Go and TypeScript suites. `go test -short ./...` skips tests
that obtain a database through `testdb.Pool`; unit tests do not start containers. The integration
and coverage commands fail if PostgreSQL containers cannot start, including outside CI.

## Browser setup

Use a disposable database. The restore tests intentionally replace their fixture household's data.
Start PostgreSQL with `task db:up`, set `DATABASE_URL` and `PUBLIC_URL=http://localhost:3000` in
`.env`, then run `task migrate` and `task e2e`. Playwright starts the API and web server. The dev-only
login is enabled by the test configuration and is disabled in production. Backup, household and
management tests have separate fixture users for each browser project.

The browser suite checks transactions, transfers, recurring entries, imports, reports, budgets,
backup/restore, household permissions and MCP consent. Management cases also verify label edits
and deletion, wallet visibility, cancelling destructive actions, invalid backup rejection and
sign-out. Setup may use the API; the behavior under test uses the browser, with API reads checking
persisted state. Async controls wait for their resulting state instead of fixed sleeps.

## Coverage and CI

Go reports are `coverage/go/coverage.out` and `coverage/go/index.html`. They instrument application
packages even when exercised through HTTP or Lambda. Generated protobuf/SQL code, command entry
points and test infrastructure are excluded from that percentage. TypeScript LCOV is written to
`coverage/js/lcov.info`; Bun measures loaded modules, so this is not whole-UI coverage. Browser
coverage is represented by behavior scenarios rather than a statement percentage.

CI runs Go unit tests, the full PostgreSQL suite with coverage, TypeScript tests with coverage,
and desktop/mobile E2E. Coverage and the HTML Playwright report are uploaded as artifacts; failing
browser tests retain traces. Open the report with `bun x playwright show-report` in `apps/web`.

These suites are regression protection, not a claim of 100% coverage. Live Google identity-provider
exchanges and production deployment behavior require separate validation. Add a failing regression
case whenever fixing a bug, and expand this scenario matrix when adding a feature.
