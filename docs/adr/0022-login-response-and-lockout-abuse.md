# ADR 0022: Login Response Uniformity and Account Lockout Abuse Mitigation

## Status
Accepted (Supersedes HTTP 423 response behavior specified in [ADR 0021](0021-account-lockout-and-advanced-rbac.md))

## Context

In [ADR 0021](0021-account-lockout-and-advanced-rbac.md), Task Manager API introduced a stateful Account Lockout policy (locking an account for `AUTH_LOCKOUT_DURATION` after `AUTH_LOCKOUT_THRESHOLD` consecutive failed login attempts within `AUTH_LOCKOUT_WINDOW`) and specified returning **HTTP `423 Locked`** (`ACCOUNT_LOCKED`) when a locked account attempts to authenticate.

Upon security review, returning a distinct HTTP `423 Locked` status code introduces two security concerns:

1. **User Enumeration Oracle**:
   - An unauthenticated attacker can probe an arbitrary list of email addresses by submitting 5+ intentionally wrong passwords.
   - Non-existent emails always return `401 Unauthorized`, whereas existing emails transition from `401 Unauthorized` to `423 Locked` on the 5th attempt.
   - Even with dummy password hashing, the HTTP status code and response body leak whether an email address is registered in the system.
2. **Account Lockout Denial of Service (DoS)**:
   - Once an attacker knows or guesses a legitimate user's email address, they can repeatedly trigger lockouts every 15 minutes to prevent the legitimate user from logging in.

We must decide how the login endpoint should behave and respond when an account is locked, balancing **anti-enumeration**, **distributed brute-force defense**, **DoS resilience**, and **legitimate user experience**.

---

## Options Considered

### Option (a): Uniform `401 Unauthorized` Response for Locked Accounts (Chosen)
When an account is currently locked (`locked_until > NOW()`), the authentication service still executes a dummy bcrypt comparison (`s.dummyHash`) to equalize execution latency, logs the blocked attempt internally, and returns the exact same **HTTP `401 Unauthorized`** (`UNAUTHORIZED` / `"invalid email or password"`) as a wrong password or non-existent email.

- **Pros**:
  - **Eliminates User Enumeration**: Responses (HTTP status, JSON body, error code) and execution time are indistinguishable across all three failure scenarios: non-existent email, wrong password, and locked account.
  - **Preserves Distributed Brute-Force Defense**: Lockout remains enforced per account regardless of how many IP addresses an attacker rotates through.
  - **Low Complexity & Stateless Response**: Requires no extra Redis/DB state beyond existing `users` columns.
- **Cons**:
  - A legitimate user whose account is temporarily locked sees `"invalid email or password"` even if they enter the correct password, unless notified out-of-band.
  - Targeted Account DoS remains possible if the attacker already knows the target's email (though the attacker receives no confirmation signal that the lockout succeeded).

### Option (b): Lockout Scoped by `(email, client_ip)` Pair
Track failed attempts and apply lockouts per `(email, IP)` tuple rather than globally on the user account.

- **Pros**:
  - Prevents an external attacker from locking out a legitimate user authenticating from a different IP address (mitigates Account DoS).
- **Cons**:
  - **Ineffective Against Distributed Botnets**: Residential proxy botnets rotate IP addresses on every request, meaning an attacker can make unlimited password guesses against a single account without ever hitting the per-`(email, IP)` threshold.
  - **Redundant with IP Rate Limiter**: Single-IP brute force is already throttled by `pkg/limiter` (ADR 0003).
  - **Increased State Footprint**: Requires unbounded tracking tables/keys per `(email, IP)` combination.

### Option (c): Progressive Delay / Exponential Backoff
Instead of locking the account, sleep on the server for progressively longer durations (e.g., 1s, 2s, 4s, 8s, 16s) on consecutive failed attempts.

- **Pros**:
  - Avoids hard account lockouts and removes the binary 401 vs. 423 signal.
- **Cons**:
  - **Server Resource Exhaustion (Slowloris-style DoS)**: Holding HTTP request goroutines and database/network resources open while sleeping allows an attacker with modest concurrency to exhaust server worker pools.
  - **Concurrency Bypass**: Attackers opening parallel connections can test multiple passwords simultaneously before backoff limits accumulate.

---

## Decision

We adopt **Option (a): Uniform `401 Unauthorized` Response** paired with **out-of-band security logging and administrative unlock capability**:

1. **Uniform API Response**:
   - In `AuthService.Login`, when `user.LockedUntil.Valid && user.LockedUntil.Time.After(time.Now())`:
     - Execute `s.hasher.ComparePassword(s.dummyHash, input.Password)` to guarantee constant bcrypt cost timing.
     - Return `ErrInvalidCredentials` (`HTTP 401 Unauthorized`, code `UNAUTHORIZED`, detail `"unauthorized: invalid email or password"`).
   - All three failure paths (non-existent email, wrong password, locked account) yield identical HTTP status, RFC 9457 JSON payload, and comparable response times.

2. **Security Logging Policy**:
   - Structured server-side logs (`slog`) MUST record security events at `WARN` level without ever logging plaintext passwords:
     - When `RecordFailedLogin` triggers a new lockout (`locked_until` becomes non-null): log `"event": "auth.account_locked"`, `user_id`, `failed_attempts`, `locked_until`, `client_ip` (if available in context).
     - When a login attempt is rejected because the account is already locked: log `"event": "auth.login_blocked_locked_account"`, `user_id`, `locked_until`.
   - These structured logs enable SIEM correlation, Prometheus/Alertmanager alerting on distributed credential-stuffing spikes, and audit investigations.

3. **Legitimate User Notification & Recovery**:
   - **Self-Healing Window**: Temporary lockouts automatically expire after `AUTH_LOCKOUT_DURATION` (default 15 minutes).
   - **Out-of-Band Notification**: Because the HTTP response intentionally omits lockout status, legitimate users should be notified via an out-of-band channel (e.g., asynchronous Email Security Alert when SMTP/notification delivery is configured in future releases) informing them of suspicious failed login attempts and the temporary 15-minute cooldown.
   - **Administrative Unlock**: Administrators can inspect security logs/audit trails and immediately clear a user's lockout via `POST /api/v1/admin/users/:id/unlock` (implemented in Phase S).

---

## Consequences

### Positive
- **Zero Email Enumeration**: Attackers gain zero signal from `/api/v1/auth/login` responses or latency measurements to confirm email existence or lockout state.
- **Full Botnet Protection**: Distributed attacks rotating thousands of IPs are still arrested at `AUTH_LOCKOUT_THRESHOLD` attempts.
- **Server Stability**: No artificial request sleeps or connection-holding delays.

### Residual Risks & Mitigations
- **Targeted Account DoS**: An attacker who already knows a victim's email can still cause a 15-minute lockout.
  - *Mitigation*: Lockout duration is bounded (`15m` default); IP-based rate limiting (`RATE_LIMIT_AUTH_*`) throttles single-source request floods; attackers receive no confirmation feedback; admins can manually unlock affected users.
- **User Confusion During Lockout**: A legitimate user entering the correct password during a 15-minute lockout receives `"invalid email or password"`.
  - *Mitigation*: Out-of-band email alerts (future milestone) and admin support unlock procedure.
