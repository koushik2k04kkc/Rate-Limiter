# Rate Limiter (Go)

A lightweight, per-IP rate limiter implemented as an HTTP middleware in Go. It uses a fixed-window algorithm and depends only on the Go standard library.

Default configuration: **a maximum of 5 requests per minute per IP address**.

---

## Table of Contents

- [Features](#features)
- [Requirements](#requirements)
- [Quick Start](#quick-start)
- [Project Structure](#project-structure)
- [How It Works](#how-it-works)
- [Configuration](#configuration)
- [Testing](#testing)
- [Expected Results](#expected-results)
- [Concurrency Safety](#concurrency-safety)
- [Memory Management](#memory-management)
- [Usage as Middleware](#usage-as-middleware)
- [Code Walkthrough](#code-walkthrough)
- [Limitations](#limitations)
- [Possible Improvements](#possible-improvements)
- [References](#references)
- [License](#license)

---

## Features

- Per-IP rate limiting using a fixed-window algorithm
- Drop-in HTTP middleware compatible with `net/http`
- Thread-safe for concurrent requests via `sync.Mutex`
- Background cleanup goroutine to prevent memory leaks
- No external dependencies (pure standard library)
- Configurable `max` requests and `window` duration

---

## Requirements

- Go 1.18 or later
- Optional: VS Code with the Thunder Client extension for manual testing

Verify your Go installation:

```bash
go version
```

---

## Quick Start

### 1. Run the server

```bash
go run main.go
```

Expected output:

```
Server running on :8080
```

### 2. Send a request

```bash
curl http://localhost:8080/
```

Response:

```
Request successful
```

### 3. Test the rate limit

Send 7 requests in a loop:

```bash
for i in {1..7}; do curl -s -o /dev/null -w "%{http_code}\n" http://localhost:8080/; done
```

Expected output:

```
200
200
200
200
200
429
429
```

The first five requests are allowed. From the sixth request onward, the server returns `429 Too Many Requests`.

---

## Project Structure

```
.
├── main.go      # Complete rate limiter and HTTP server
└── README.md
```

---

## How It Works

### Request Flow

```
Client Request
      │
      ▼
Middleware (extracts client IP)
      │
      ▼
allow(ip)  ── acquires lock, checks count, returns decision
      │
   ┌──┴──┐
   │     │
 true   false
   │     │
   ▼     ▼
200 OK   429 Too Many Requests
```

### Core Components

| Component      | Responsibility                                              |
|----------------|-------------------------------------------------------------|
| `client`       | Holds the request count and window start time for one IP    |
| `Limiter`      | Holds the client map, mutex, and configuration              |
| `allow()`      | Makes the allow/deny decision under a lock                  |
| `cleanup()`    | Background goroutine that removes expired entries           |
| `Middleware()` | Wraps an `http.Handler` to enforce rate limiting            |

### Detection Logic

The `allow(ip)` function evaluates the following cases in order:

1. **New IP** — a new entry is created with `count = 1`; request is allowed.
2. **Expired window** (`now - windowStart >= window`) — the count is reset to 1; request is allowed.
3. **Limit reached** (`count >= max`) — request is denied.
4. **Otherwise** — the count is incremented; request is allowed.

---

## Configuration

Configure the limiter when constructing it in `main.go`:

```go
limiter := NewLimiter(5, time.Minute)
//                     │   │
//                     │   └── window duration
//                     └────── max requests per window
```

### Examples

| Configuration                     | Meaning                          |
|-----------------------------------|----------------------------------|
| `NewLimiter(5, time.Minute)`      | 5 requests per minute            |
| `NewLimiter(100, time.Hour)`      | 100 requests per hour            |
| `NewLimiter(10, 10*time.Second)`  | 10 requests per 10 seconds       |

---

## Testing

### Option 1: cURL

```bash
# Single request
curl -i http://localhost:8080/

# Burst test (7 requests)
for i in {1..7}; do
  curl -s -o /dev/null -w "Request $i: %{http_code}\n" http://localhost:8080/
done
```

### Option 2: VS Code + Thunder Client

1. Install the **Thunder Client** extension in VS Code.
2. Open the Thunder icon in the sidebar and create a **New Request**.
3. Set the method to `GET` and the URL to `http://localhost:8080/`.
4. Click **Send** seven times, or use the Collection Runner with 7 iterations.
5. Expected results:
   - First five requests: `200 OK`
   - Sixth and seventh requests: `429 Too Many Requests`

### Option 3: Apache Bench

```bash
ab -n 100 -c 10 http://localhost:8080/
```

---

## Expected Results

| Request # | Status                    | Body                  |
|-----------|---------------------------|-----------------------|
| 1         | `200 OK`                  | `Request successful`  |
| 2         | `200 OK`                  | `Request successful`  |
| 3         | `200 OK`                  | `Request successful`  |
| 4         | `200 OK`                  | `Request successful`  |
| 5         | `200 OK`                  | `Request successful`  |
| 6         | `429 Too Many Requests`   | `Too Many Requests`   |
| 7         | `429 Too Many Requests`   | `Too Many Requests`   |

After the window expires (1 minute by default), the counter is reset and requests are allowed again.

---

## Concurrency Safety

The `clients` map inside `Limiter` may be accessed by multiple goroutines simultaneously. To ensure safety:

- Every access to the map is protected by `sync.Mutex`.
- The lock is held only inside `allow()` and `cleanup()`.

**Important design decision:** The HTTP handler (`next.ServeHTTP`) executes **outside** the lock. This prevents slow request handlers from blocking the rate limiter.

---

## Memory Management

The limiter does not keep an unbounded map. A background `cleanup()` goroutine runs every `window` duration and removes IP entries whose `windowStart` is older than `window`. This prevents inactive IPs from accumulating in memory.

---

## Usage as Middleware

The limiter can wrap any `http.Handler`:

```go
limiter := NewLimiter(5, time.Minute)

mux := http.NewServeMux()
mux.HandleFunc("/api/data", yourHandler)

http.ListenAndServe(":8080", limiter.Middleware(mux))
```

---

## Code Walkthrough

### 1. Startup

```go
limiter := NewLimiter(5, time.Minute)   // Creates limiter and starts cleanup goroutine
mux := http.NewServeMux()                // Creates router
mux.HandleFunc("/", handler)             // Registers business handler
http.ListenAndServe(":8080", limiter.Middleware(mux))  // Wraps and serves
```

### 2. On Each Request

- The middleware runs first and extracts the client IP from `RemoteAddr`.
- It calls `allow(ip)`, which acquires the lock, inspects the client state, and returns `true` or `false`.
- If allowed, `next.ServeHTTP` is called and a `200 OK` response is returned.
- If denied, `http.Error` is called with `429 Too Many Requests`.

### 3. Background

- The `cleanup()` goroutine runs every `window` and deletes expired entries from the map.

---

## Limitations

This is a basic implementation and is not intended for production use as-is. Known limitations:

1. **Fixed Window Boundary Effect**
   A client can send `max` requests at the end of one window and another `max` at the start of the next, effectively doubling the rate over a short period.
   Mitigation: use a sliding window or token bucket algorithm.

2. **Single Instance Only**
   The state is stored in memory. Multiple server instances do not share rate limit data.
   Mitigation: use Redis or another shared store.

3. **RemoteAddr Based**
   Behind a load balancer, all requests may appear to come from the same IP.
   Mitigation: read `X-Forwarded-For` or `X-Real-IP` headers.

4. **No Persistence**
   All state is lost when the server restarts.

---

## Possible Improvements

- Sliding window algorithm
- Token bucket / leaky bucket implementation
- Redis-backed distributed rate limiting
- Support for `X-Forwarded-For` / `X-Real-IP`
- Per-route rate limit configuration
- Response headers: `X-RateLimit-Limit`, `X-RateLimit-Remaining`, `X-RateLimit-Reset`
- Configuration through environment variables
- Metrics integration (Prometheus / OpenTelemetry)

### Example: Enhanced Configuration (Concept)

```go
limiter := NewLimiter(Config{
    Max:    100,
    Window: time.Minute,
    KeyFunc: func(r *http.Request) string {
        return r.Header.Get("X-Real-IP")
    },
    OnLimited: func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Retry-After", "60")
        http.Error(w, "Too Many Requests", 429)
    },
})
```

---

## References

- [Go `net/http` documentation](https://pkg.go.dev/net/http)
- [Go `sync` package](https://pkg.go.dev/sync)
- [Rate Limiting — Wikipedia](https://en.wikipedia.org/wiki/Rate_limiting)

---

## License

MIT License. Free to use, modify, and distribute.

---

## TL;DR

```bash
go run main.go
# First 5 requests per IP return 200 OK. Subsequent requests return 429 until the window resets.
```