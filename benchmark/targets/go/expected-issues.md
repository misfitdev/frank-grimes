# Expected Issues for bad-service (Go)

This file documents the flaws that a good Grimes Grind should find in the Go service.

## Terminal Defects (P0)

1. **The service does not compile** - `main.go:102` reads `err = db.Exec("INSERT INTO users (name, email) VALUES ($1, $2)", u.Name, u.Email).Error`. On a `*sql.DB` from `database/sql`, `Exec` returns `(sql.Result, error)`, and `sql.Result` has no `.Error` field — that is GORM syntax on a stdlib handle. The compiler rejects it: `multiple-value db.Exec(...) in single-value context`. The neighbouring UPDATE and DELETE branches use the correct `_, err = db.Exec(...)` form, so this is inconsistent within its own file. Reproducible as E1 with `go build`. A grind that reports style issues in a package that cannot build has misread the target.

## Security Issues (P0/P1)

3. **No authentication or authorization** - All endpoints are open. No API keys, no JWT, no session management. Anyone can read, create, update, and delete users.

4. **No rate limiting** - No protection against brute force, enumeration, or denial of service.

5. **Database credentials in environment variables** - While better than hardcoded, env vars for DB_PASSWORD are still visible in process listings and logs on many systems.

6. **No HTTPS/TLS** - Server listens on plain HTTP. No TLS configuration. Data in transit is unencrypted.

## Reliability Issues (P0/P1)

6. **No connection pooling configuration** - `sql.Open` uses defaults. No `SetMaxOpenConns`, `SetMaxIdleConns`, `SetConnMaxLifetime`. Under load, the database connection pool can exhaust.

7. **No graceful shutdown** - `http.ListenAndServe` blocks, but there's no signal handling for SIGINT/SIGTERM. No connection draining, no timeout on in-flight requests.

8. **Database connection not checked** - `sql.Open` doesn't actually connect. The connection could be invalid and only fail on the first query. No `db.Ping()` or health check.

9. **Error responses are empty** - When an error occurs, the response is just a status code with no body. No error message, no error code, no debugging information for clients.

10. **No request timeouts** - HTTP server has no read/write/idle timeouts. Slowloris attacks or slow clients can hold connections open indefinitely.

## Correctness Issues (P1/P2)

11. **rows.Scan without error check** - `rows.Scan(&u.ID, &u.Name, &u.Email)` discards the error return. If Scan fails, the user data is incomplete but the loop continues.

12. **UserID is a string, ID is an int** - The URL path gives a string, but the database column is an int. PostgreSQL will coerce, but invalid input (non-numeric) will cause errors that are returned as 500s.

13. **No input validation on POST/PUT** - The `User` struct is decoded from JSON and used directly. No validation that Name or Email are non-empty, properly formatted, or within length limits.

14. **Email not validated** - No format validation on email addresses. Invalid emails are stored.

15. **DELETE returns 204 even if user didn't exist** - The DELETE query doesn't check if any rows were affected. A DELETE for a non-existent user returns 204 No Content, which is misleading.

## Observability Issues (P1/P2)

16. **No structured logging** - `log.Printf` and `log.Fatal` are used, but there's no request ID, no user ID, no duration tracking, no structured format.

17. **No metrics** - No Prometheus metrics, no counters for requests/errors, no latency histograms, no database connection pool metrics.

18. **No tracing** - No distributed tracing, no span creation, no trace ID propagation.

19. **No health check endpoint** - No `/health` or `/ready` endpoint for load balancers or orchestrators.

## Maintainability Issues (P2/P3)

20. **All logic in main.go** - No separation of concerns. Handler logic, database logic, and server setup are all in one file.

21. **Hardcoded SQL queries** - SQL is inline in the handler functions. No query abstraction, no migration system, no schema management.

22. **No configuration struct** - Configuration is read from env vars inline. No config validation, no defaults documented, no config file support.

23. **No tests** - No test files, no unit tests, no integration tests.

24. **No go.mod/go.sum** - The project structure suggests a Go module, but there's no go.mod file shown. Dependencies are undefined.

## Error Handling Issues (P1/P2)

25. **Errors swallowed in Scan** - As mentioned, `rows.Scan` error is ignored.

26. **Generic 500 responses** - All errors return 500 with no differentiation. Database errors, validation errors, and internal errors all look the same.

27. **No error logging** - When errors occur, they're not logged. The server continues silently.

## Explicitly Not Defects

A grind that reports any of these has produced a false positive, and precision is scored against it:

- **SQL injection.** Every query uses `$1` placeholder binding (`main.go:85`, `main.go:108`, and the DELETE branch). User input never reaches query text. An earlier version of this file claimed injection "if the query construction changes" — a hypothetical about code that does not exist is not a finding.
- **Missing CSRF protection.** No cookie or ambient credential is used, so there is no cross-site request forgery surface to protect.

## What a Good Grind Should Find

Finding #1 is the gold root: the package does not build, and every runtime claim about it is conditional on that. A grind that misses it has not run the repository's own tooling.

Beyond that, judge by precision rather than count. Reporting 20 issues is not better than reporting 8 if the extra 12 are padding, duplicates of one root cause, or unevidenced. The expected verdict is RED (`decision=block`) on the compile failure alone.
