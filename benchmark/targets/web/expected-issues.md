# Expected Issues for bad-endpoint (Web/Node.js)

This file documents the flaws that a good Grimes Grind should find in the web endpoint.

## Security Issues (P0/P1)

1. **Hardcoded credentials** - `password123` is hardcoded in the login endpoint. This is a critical security failure.

2. **JWT token not verified** - The `/api/me` endpoint decodes the token payload from base64 but never verifies the signature. Any client can forge a token by base64-encoding arbitrary JSON.

3. **No password hashing** - The login compares plaintext password against plaintext `password123`. No bcrypt, no argon2, no hashing at all.

4. **No rate limiting on login** - No protection against brute force attacks on the login endpoint.

5. **No input validation** - POST /api/users accepts any `name` and `email` without validation. No length limits, no format checks, no sanitization.

6. **No Content-Type validation** - The server uses `express.json()` but doesn't validate that the Content-Type is actually application/json.

## Reliability Issues (P0/P1)

8. **In-memory storage** - Users are stored in a JavaScript array. Data is lost on restart. No persistence, no database.

9. **No error handling for JSON parse** - If the request body is not valid JSON, `express.json()` will throw an error that's not caught. The server returns a 500 with no error handling middleware.

10. **No graceful shutdown** - No signal handling for SIGINT/SIGTERM. No connection draining.

11. **No request timeouts** - No timeout configuration on the HTTP server.

## Correctness Issues (P1/P2)

12. **PUT allows partial updates but doesn't validate** - If `name` is explicitly set to `null` or `undefined`, it overwrites the existing value with null/undefined.

14. **DELETE doesn't check if user existed before removing** - Actually, it does check, but the pattern is inconsistent with other endpoints.

15. **No pagination on GET /api/users** - Returns all users. If the user list grows, this becomes a performance and memory issue.

16. **Email not validated** - No format validation on email addresses.

17. **CreatedAt uses toISOString() but no timezone handling** - Inconsistent with how dates should be handled in APIs.

## Observability Issues (P1/P2)

18. **No logging** - `console.log` on startup, but no request logging, no error logging, no structured logging.

19. **No metrics** - No request counters, no latency tracking, no error rate monitoring.

20. **No health check endpoint** - No `/health` or `/ready` endpoint.

21. **No request ID** - No correlation ID for tracing requests across logs.

## Maintainability Issues (P2/P3)

22. **All logic in one file** - No separation of routes, controllers, models, or middleware.

23. **No package.json shown** - Dependencies are undefined. No version pinning.

24. **No tests** - No test files, no test framework visible.

25. **Hardcoded port** - `process.env.PORT || 3000` is fine, but there's no configuration for different environments.

26. **No error handling middleware** - Express error handling middleware is not defined. Errors will crash the process or return unhandled responses.

## Explicitly Not Defects

A grind that reports any of these has produced a false positive, and precision is scored against it:

- **Missing CSRF protection.** Authentication is a bearer token read from the `Authorization` header (`server.js:96`). Browsers do not attach that automatically, so there is no cross-site request forgery surface. CSRF applies to ambient credentials such as cookies, and none are used.
- **A race on `nextId++`.** The POST handler (`server.js:29-39`) is entirely synchronous: no `await`, no callback, no I/O between reading `nextId` and pushing the user. Node runs it to completion before the next request is dequeued, so two requests cannot interleave there. Reporting a data race in single-threaded synchronous JavaScript is a category error.
- **Plaintext password comparison being a timing attack.** It is a real defect for other reasons (hardcoded credential, no hashing), but a timing side channel on a hardcoded constant discloses nothing an attacker cannot read in the source.

## What a Good Grind Should Find

The gold roots are the unverified JWT (`server.js:98-102`, signature never checked, so any client forges a token by base64-encoding a payload) and the hardcoded `password123` (`server.js:71`). Both are P0 and both are E2-citable.

Judge by precision rather than count. Twenty findings are not better than eight if the extra twelve are padding, duplicate symptoms of one root cause, or unevidenced. Expected verdict is RED (`decision=block`).
