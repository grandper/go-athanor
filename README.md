# go-athanor

[![Test](https://github.com/grandper/go-athanor/actions/workflows/go-test.yml/badge.svg)](https://github.com/grandper/go-athanor/actions/workflows/go-test.yml)
[![Lint](https://github.com/grandper/go-athanor/actions/workflows/go-lint.yml/badge.svg)](https://github.com/grandper/go-athanor/actions/workflows/go-lint.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/grandper/go-athanor/app.svg)](https://pkg.go.dev/github.com/grandper/go-athanor/app)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

**athanor** — compose Go applications from processes and resources.

```go
err := app.Run(ctx,
	app.WithResource(databasePool),
	app.WithResource(httpserver.NewServer(config, apiHandler)),
	app.WithProcess(process.ListenToSignals),
)
```

Wiring an application together is the part every Go service rewrites: start the
servers, tie their lifetimes to a single context, tear them down in the right
order, and turn a POSIX signal into a clean exit rather than a stack trace.
`athanor` does that for you, so your own code is left holding nothing but the
business logic.

You describe the application as managed resources, background processes, and
hooks. `Run` initializes them, supervises them concurrently, and shuts them all
down gracefully on the first failure, on a signal, or when the context is
canceled. On top of that runtime, the library ships the servers (HTTP, gRPC,
NATS) as managed resources, each populated by your own handlers.

## Why athanor?

An athanor is the alchemist's furnace: a self-feeding tower of charcoal that
held a constant temperature for weeks, unattended, so the transformation could
run to completion. Alchemists called it *Slow Henry*.

It never did the work itself. It held the conditions under which the work could
happen — which is exactly what this library does for your processes.

**Main features:**

- An application runner (`app.Run`) that initializes your resources, runs
  them concurrently with your background processes, and shuts everything down
  gracefully on the first failure, on a POSIX signal, or when the context is
  canceled.
- Before and after hooks, and custom processes managed alongside the resources.
- Unified lifecycle management for the technical resources your app depends on:
  initialization, observable health statuses, rollback, and failure-driven shutdown.
- HTTP, gRPC, and NATS servers that are managed resources, each populated by
  pluggable handlers.
- A standalone runner (`resource.Run`) for the programs made of a single
  resource, such as one server.
- A ready-made `HealthHandler` for every server, reporting the health of your
  resources over HTTP, the gRPC health checking protocol, and NATS, plus
  handlers for version reporting and static files.
- Response helpers for every server, speaking the vocabulary of their transport:
  HTTP status codes over HTTP and NATS, gRPC codes over gRPC.

## Installation

```bash
go get github.com/grandper/go-athanor
```
The library requires Go 1.21 or later.

## Packages at a glance

The library is split into small packages named after their concept. Several of
them are named after a protocol (`http`, `grpc`, `nats`), so the examples in
this README import them under explicit aliases:

```go
import (
	"github.com/grandper/go-athanor/app"
	"github.com/grandper/go-athanor/process"
	"github.com/grandper/go-athanor/resource"
	grpcserver "github.com/grandper/go-athanor/resource/server/grpc"
	httpserver "github.com/grandper/go-athanor/resource/server/http"
	natsserver "github.com/grandper/go-athanor/resource/server/nats"
)
```

| Package | What it gives you |
| --- | --- |
| `app` | `Run`, the application runner, and its options. |
| `process` | The POSIX signal listener. |
| `resource` | The `Managed` interface, observable statuses, the `List` driving resources as a group, the standalone `Run`, and cleanup contexts. |
| `resource/server/http` | The HTTP server resource, its handlers, and its response helpers. |
| `resource/server/grpc` | The gRPC server resource, its health handler, and its error response helper. |
| `resource/server/nats` | The NATS server resource, its health handler, and its response helpers. |

## Quick start

A complete service serving a greeting over HTTP until it receives SIGINT or
SIGTERM:

```go
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"

	"github.com/gorilla/mux"

	"github.com/grandper/go-athanor/app"
	"github.com/grandper/go-athanor/process"
	httpserver "github.com/grandper/go-athanor/resource/server/http"
)

type HelloHandler struct{}

func (h *HelloHandler) Register(router *mux.Router) error {
	router.HandleFunc("/hello", h.handleHello).Methods(http.MethodGet)
	return nil
}

func (h *HelloHandler) handleHello(w http.ResponseWriter, _ *http.Request) {
	_ = httpserver.RespondWithJSON(w, http.StatusOK, map[string]string{"message": "Hello, World!"})
}

func run(ctx context.Context) error {
	config, err := httpserver.NewConfigFromEnv()
	if err != nil {
		return err
	}

	return app.Run(ctx,
		app.WithResource(httpserver.NewServer(config, &HelloHandler{})),
		app.WithProcess(process.ListenToSignals),
	)
}

func main() {
	if err := run(context.Background()); err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		os.Exit(1)
	}
}
```

Keeping `main` down to wiring and delegating to a `run` function keeps the
application itself testable: a test calls `run` with a context it cancels.

## Running an application

The `app` package wraps everything an application does at runtime behind a
single call to `app.Run`. It takes exactly three kinds of thing — managed
resources, hooks, and processes — and knows nothing about any particular
transport: an HTTP, gRPC, or NATS server is just one more resource.

You describe the application with functional options: each `app.Option`
configures the `app.App` that `Run` builds and runs for you. `Run` runs the before
hooks, initializes the resources, runs them concurrently with your processes,
waits for the first failure, a termination signal, or the cancellation of the
context, shuts everything down gracefully, and finally runs the after hooks:

```go
err := app.Run(ctx,
	app.WithHookBefore(migrateDatabase),
	app.WithResources(databasePool, httpServer),
	app.WithProcesses(process.ListenToSignals, pollOutbox),
	app.WithHookAfter(flushTelemetry),
)
```

### Registering resources

A resource is anything implementing `resource.Managed` (see
[Resource management](#resource-management)). You build it where you wire your
dependencies, and hand `app.Run` the finished object:

```go
app.WithResource(databasePool)                          // one resource
app.WithResources(httpServer, grpcServer, natsServer) // several at once
```

Registered resources are initialized in registration order, before any process
starts, and shut down in reverse order once every process has stopped — so
register a dependency, such as a database pool, before the servers that use it.
Registering two resources with the same `Name` fails with
`resource.ErrAlreadyRegistered`. When a resource
fails to initialize, `Run` rolls back the resources it already started, reports
the error, and still runs the after hooks.

### Hooks

A hook is an `app.ProcessFunc` that runs once instead of for the whole life
of the application:

```go
type ProcessFunc func(ctx context.Context) error
```

Before hooks run sequentially before the resources are initialized, and the
first failing hook aborts the run. After hooks run sequentially once everything
has stopped — even when a process or a resource failed — so they can release
whatever the resources depended on. After hooks are cleanup: every one of them
runs, even when an earlier one failed, and `Run` returns all their failures
joined:

```go
err := app.Run(ctx,
	app.WithHookBefore(openDatabase),
	app.WithHooksBefore(loadSecrets, warmCache),
	app.WithHookAfter(closeDatabase),
	app.WithHooksAfter(flushMetrics, flushTraces),
)
```

Before hooks receive the context you passed to `Run`. After hooks receive a
context that keeps its values (a logger, a trace) but not its cancellation —
the cancellation of your context is often the very thing that stopped the app,
and a cleanup must not start already canceled. That context is bounded instead:
the after hooks share 30 seconds to finish.

```go
err := app.Run(ctx, app.WithHooksAfter(closeDatabase, flushMetrics))

errors.Is(err, errDatabase) // true when closeDatabase failed...
errors.Is(err, errMetrics)  // ...and flushMetrics still ran, and failed too
```

### Custom processes

A process is also an `app.ProcessFunc`, but it runs concurrently with the
resources for as long as the application lives. It must return promptly when
its context is canceled:

```go
err := app.Run(ctx,
	app.WithProcess(func(ctx context.Context) error {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return nil
			case <-ticker.C:
				// periodic work
			}
		}
	}),
)
```

Use `WithProcesses` to register several processes at once.

### POSIX signals and graceful shutdown

The signal listener is an ordinary process: `process.ListenToSignals` already
has the `app.ProcessFunc` signature, so you register it like any other:

```go
err := app.Run(ctx, app.WithProcess(process.ListenToSignals))
```

It listens for SIGINT, SIGTERM, and SIGQUIT, and reports a received signal as
`process.ErrPOSIXSignalReceived`. That error cancels every other resource and
process, and `Run` maps it back to `nil` once everything has stopped: a signal
is a request for graceful shutdown, not an error. Any process may ask for a
graceful stop the same way by returning that sentinel.

When you already own a signal channel, `process.WaitForSignal` blocks until a
signal arrives on it and reports it the same way:

```go
signals := make(chan os.Signal, 1)
signal.Notify(signals, syscall.SIGHUP)

err := process.WaitForSignal(ctx, signals) // errors.Is(err, process.ErrPOSIXSignalReceived)
```

Everything else stops the app the same way, through a single shared context:
a failing resource, a failing process, or the cancellation of the context you
passed to `Run` cancels the others. The resources are then shut down with the
same kind of context as the after hooks — the values of yours, not its
cancellation, bounded to 30 seconds — so the graceful stop is not cut short by
the cancellation that triggered it. `Run` returns `nil` for a cancellation or a
signal, and the error when a hook, a resource, or a process failed — including
a resource that fails to shut down after a signal: only the signal itself is
forgiven. The error
is wrapped with what failed (`before hook failed: ...`), so match its cause
with `errors.Is`.

## Resource management

The `resource` package provides unified lifecycle management for the technical
resources your app depends on (database pools, message brokers, caches,
servers...): initialization, health-status tracking, and failure-driven shutdown.

### Implementing a resource

A resource implements the single `Managed` interface:

```go
type Managed interface {
	Name() string
	Status() *ObservableStatus
	Init(ctx context.Context) error
	Shutdown(ctx context.Context) error
}
```

It owns its status as an `*resource.ObservableStatus`, created at construction
so it is available before `Init`, and it is the sole writer of that status:

```go
type database struct {
	status *resource.ObservableStatus
}

func newDatabase() *database {
	return &database{status: resource.NewObservableStatus(resource.StatusInitializing)}
}

func (d *database) Name() string                       { return "database" }
func (d *database) Status() *resource.ObservableStatus { return d.status }

func (d *database) Init(ctx context.Context) error {
	// ... open the connection pool, ping the server ...
	d.status.Set(resource.StatusHealthy)
	return nil
}

func (d *database) Shutdown(ctx context.Context) error {
	// ... drain and close the connection pool ...
	d.status.Set(resource.StatusClosed)
	return nil
}
```

### Statuses

A `Status` is one of:

| Status | Meaning |
| --- | --- |
| `StatusInitializing` | The resource is starting up. This is the zero value. |
| `StatusHealthy` | The resource is fully operational. |
| `StatusDegraded` | The resource is operational but experiencing issues. |
| `StatusUnhealthy` | The resource is not operational, but may recover. |
| `StatusFailed` | The resource failed unrecoverably; this triggers the shutdown of the app. |
| `StatusClosed` | The resource was intentionally shut down. |

`String` prints a status as its upper-case name (`HEALTHY`, `FAILED`...), or
`UNKNOWN` for a value outside the list. A status also answers the questions you
usually ask about it:

```go
status.IsOperational() // true for StatusHealthy and StatusDegraded
status.IsFailed()      // true for StatusFailed
status.IsTerminal()    // true once the resource has stopped for good
```

### Observing a status

An `ObservableStatus` is safe for concurrent use. Anyone holding it can read
the current value with `Get` and subscribe to changes with `Observe`, which
returns the function that removes the observer. Observers are notified
synchronously, and only when the value actually changes. An observer is
anything implementing the single-method `StatusObserver` interface, whose
`StatusChangedTo` receives the new status. Plain functions become observers
with `StatusObserverFunc`:

```go
stop := db.Status().Observe(resource.StatusObserverFunc(func(s resource.Status) {
	slog.Info("database status changed", "status", s)
}))
defer stop()

current := db.Status().Get()
```

Only the owning resource calls `Set`.

### Managing resources with a List

To manage resources as a group, register them on a `List`. The List initializes
resources in registration order and shuts them down in reverse order. When a
resource fails to initialize, the already-started resources are rolled back
automatically:

```go
resources := resource.NewList()

if err := resources.Register(newDatabase()); err != nil {
	return err // resource.ErrAlreadyRegistered: the name is already taken
}

if err := resources.Init(ctx); err != nil {
	return err // already-started resources were rolled back
}
defer resources.Shutdown(ctx)
```

The rollback shuts the started resources down with a context of its own,
bounded to 30 seconds, which keeps the values of the `Init` context but not its
cancellation: an `Init` interrupted by a canceled context still gets a proper
cleanup.

`Init` closes the registration. A resource registered later would never be
initialized nor observed, so `Register` refuses it with
`resource.ErrListAlreadyInitialized`:

```go
err := resources.Register(newCache()) // after Init
errors.Is(err, resource.ErrListAlreadyInitialized) // true
```

`Shutdown` shuts every resource down, even when one of them fails, and returns
the first error. `Manage` blocks until a resource reports `StatusFailed`
(returned as `resource.ErrResourceFailed`, wrapped with the name of the
resource) or the context is canceled, so you can run it from the
goroutine that owns your app's lifetime — which is exactly what `app.Run`
does for you:

```go
if err := resources.Manage(ctx); err != nil {
	return err // resource.ErrResourceFailed, or the error of the canceled ctx
}
```

### Running a resource standalone

When a single resource is your whole program, you do not need `app.Run`.
`resource.Run` initializes the resource, blocks until the context is canceled
or the resource reports `StatusFailed`, then shuts it down with a
[cleanup context](#cleanup-contexts). It works with the three servers and with
any `Managed` of your own:

```go
ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
defer stop()

err := resource.Run(ctx, httpserver.NewServer(config, handlers...))
```

A canceled context is a graceful stop, so `Run` returns `nil`. It returns the
error of `Init` when the resource cannot start, an error wrapping
`resource.ErrResourceFailed` when the resource failed while running, and the
error of `Shutdown` when it cannot stop — joined when both happen, so match
them with `errors.Is`.

### Cleanup contexts

The after hooks, the shutdown of the resources, the rollback, and
`resource.Run` all derive their context the same way, with `resource.CleanupContext`. It keeps the values
of your context (a logger, a trace) but not its cancellation, and it expires
after `resource.CleanupTimeout` (30 seconds) so a cleanup that hangs cannot
block forever. You can use it for the cleanups of your own processes:

```go
func serve(ctx context.Context) error {
	<-ctx.Done() // ctx is canceled from here on

	cleanupCtx, cancel := resource.CleanupContext(ctx)
	defer cancel()
	return flush(cleanupCtx)
}
```

### The aggregate status of a List

A List is itself a `Managed` resource: `Status()` returns the List's own
observable status, re-derived from the resources' statuses when a resource is
registered, and then whenever one of them changes from `Init` until `Shutdown`.
Resources may change concurrently: the aggregate always settles on the latest
statuses, and an observer of the aggregate may itself change a resource.
The rules, in decreasing order of severity:

1. `FAILED` if any resource failed,
2. `UNHEALTHY` if any resource is unhealthy,
3. `INITIALIZING` if any resource is still initializing (or if the List is empty),
4. `DEGRADED` if any resource is degraded, or if only some resources are closed,
5. `CLOSED` if every resource is closed,
6. `HEALTHY` otherwise.

You observe the aggregate status like any other:

```go
resources.Status().Observe(resource.StatusObserverFunc(func(s resource.Status) {
	fmt.Fprintf(os.Stdout, "resources are now %s\n", s)
}))
```

Because a List is a resource, a List can be nested inside another List, or
registered in `app.Run`, as a single composite resource:

```go
err := app.Run(ctx, app.WithResource(resources))
```

`Name` always returns `"resources"`, and names are unique within a List, so a
parent holds at most one nested List.

You can also inspect individual resources:

```go
obs, ok := resources.StatusOf("database") // one resource's observable status
summary := resources.StatusSummary()      // snapshot of every resource's status
healthy := resources.IsHealthy()          // true if every resource is operational
```

## The HTTP server

The `resource/server/http` package provides the HTTP server, its handlers, and
the helpers to write JSON responses. Routing is backed by
[gorilla/mux](https://github.com/gorilla/mux).

### The HTTP server as a managed resource

`NewServer` builds the server from a `Config` and a variadic list of handlers.
The resulting `Server` implements `resource.Managed`, so you register it on
`app.Run` or on a `resource.List` like any other resource:

```go
server := httpserver.NewServer(config, &HelloHandler{})

err := app.Run(ctx, app.WithResource(server))
```

`Init` registers the handlers, binds the listener (so an occupied port fails
initialization immediately), and serves in the background.
`Shutdown` lets pending requests complete within the context's deadline. The
status moves from `StatusInitializing` to `StatusHealthy` once the listener is
bound, to `StatusFailed` when the serve loop fails, and to `StatusClosed` after
a graceful shutdown.

#### A server starts once

A server is a single-use value: it goes through `Init` and `Shutdown` once, and
that is by design. The three servers (HTTP, gRPC, NATS) follow the same rule:

- Calling `Init` on a running server fails with `ErrServerAlreadyStarted` and
  leaves the running server untouched.
- Calling `Init` on a server that was shut down fails with
  `ErrServerAlreadyStarted` too. A server cannot be restarted, and its status
  stays `StatusClosed`.
- An `Init` that failed (an occupied port, say) claimed nothing, so it can be
  retried on the same server.

```go
server := httpserver.NewServer(config, &HelloHandler{})

_ = server.Init(ctx)
_ = server.Shutdown(ctx)

err := server.Init(ctx) // httpserver.ErrServerAlreadyStarted
```

To serve again after a shutdown, build a new server with `NewServer`: the
configuration and the handlers can be reused as they are.

Inside a List the server is named `http-server` (`httpserver.DefaultName`).
Rename it with `WithName` to run several instances side by side:

```go
public := httpserver.NewServer(publicConfig, apiHandler).WithName("public-http")
admin := httpserver.NewServer(adminConfig, adminHandler).WithName("admin-http")
```

### Running the server standalone

When you do not need `app.Run`, hand the server to `resource.Run` (see
[Running a resource standalone](#running-a-resource-standalone)): it
initializes the server, serves until the context is canceled or the server
fails, then shuts down gracefully. A graceful shutdown is not an error:

```go
err := resource.Run(ctx, httpserver.NewServer(config, handlers...))
```

### Configuration

`Config` carries the bind interface, the port, and the optional TLS files. You
can fill it in yourself:

```go
config := &httpserver.Config{
	BindInterface: "127.0.0.1",
	BindPort:      8443,
	TLSCertFile:   "server.crt",
	TLSKeyFile:    "server.key",
}
```

Or read it from the environment with `NewConfigFromEnv`. The variable names and
the defaults are exported as constants:

| Environment variable | Constant | Default |
| --- | --- | --- |
| `HTTP_BIND_INTERFACE` | `BindInterfaceEnvVar` | `0.0.0.0` (`DefaultBindInterface`) |
| `HTTP_BIND_PORT` | `BindPortEnvVar` | `8080` (`DefaultBindPort`) |
| `HTTP_TLS_CERT_FILE` | `TLSCertFileEnvVar` | *(none)* |
| `HTTP_TLS_KEY_FILE` | `TLSKeyFileEnvVar` | *(none)* |

A variable that is not set falls back to its default. A `HTTP_BIND_PORT` that
is not a number between 1 and 65535 is a mistake rather than a missing value,
so `NewConfigFromEnv` fails with `ErrInvalidBindPort` instead of silently
serving on the default port.

The server serves TLS as soon as both TLS files are set:

```go
config, err := httpserver.NewConfigFromEnv()
if err != nil {
	return err // httpserver.ErrInvalidBindPort
}
config.Address()        // "0.0.0.0:8080"
config.TLSIsAvailable() // true when both TLS files are configured
```

`Init` fails with `ErrEmptyBindInterface` when the bind interface is empty, and
with `ErrBindPortNotInitialized` when the port is missing.

The server reads and writes with a 2-second timeout each, and closes idle
connections after 3 seconds, so keep long-running work out of the request.

### Writing a handler

Each server type defines a one-method `Handler` interface that your handlers
implement to wire themselves into their server. For the HTTP server, a handler
registers its routes on the router:

```go
type Handler interface {
	Register(router *mux.Router) error
}
```

The three servers share that shape: `Register` returns an error, and a handler
that cannot register makes `Init` fail with that error, wrapped in
`failed to register an HTTP handler`.

```go
type HelloHandler struct{}

func (h *HelloHandler) Register(router *mux.Router) error {
	router.HandleFunc("/hello", h.handleHello).Methods(http.MethodGet)
	return nil
}

func (h *HelloHandler) handleHello(w http.ResponseWriter, _ *http.Request) {
	_ = httpserver.RespondWithJSON(w, http.StatusOK, map[string]string{"message": "Hello, World!"})
}
```

Because the handlers go to their own server's constructor, passing a gRPC
handler to the HTTP server is a compile error.

### Responding to requests

The response helpers write a JSON body with the status code you give them and
set `Content-Type: application/json`:

```go
httpserver.RespondWithJSON(w, http.StatusOK, account)             // any value, through encoding/json
httpserver.RespondWithProtoJSON(w, http.StatusOK, accountMessage) // a proto.Message, through protojson
httpserver.RespondWithRawJSON(w, http.StatusOK, []byte(`{"key":"value"}`))
httpserver.RespondWithError(w, http.StatusBadRequest, err)
httpserver.RespondWithStatus(w, http.StatusOK, "Success")
```

`RespondWithRawJSON` takes JSON you already hold as bytes, validates it, and
trims the surrounding whitespace. `RespondWithError` and `RespondWithStatus`
both write the standard `StatusResponse` body, the first from an error and the
second from a message:

```json
{"code": 400, "status_text": "Bad Request", "message": "owner is required"}
```

You can build that body yourself with `NewStatusResponse(code, message)` and
send it with its `WriteJSON(w)` method. When you only need the text of a status
code, `WriteStatusText(w, http.StatusNotFound)` answers `Not Found` as plain
text.

A body that cannot be encoded, or raw data that is not valid JSON, is answered
with a `500` error body and fails with `httpserver.ErrJSONEncoding`. A response
that cannot be written fails with `httpserver.ErrWriteBody`.

### Version and static handlers

The package ships two ready-made handlers:

- `VersionHandler` exposes the build information you pass it on
  `GET /api/{version}/version`.
- `StaticHandler` serves the files of a directory under a URL prefix.

```go
server := httpserver.NewServer(config,
	httpserver.NewVersionHandler("v1", httpserver.VersionInfo{
		Version: "1.2.3", Commit: "abc123", BuildDate: "2026-08-10T12:00:00Z",
	}),
	httpserver.NewStaticHandler("./public", "/assets"),
)
```

### Exposing the health of your resources

`HealthHandler` reports the health of a `resource.List` over HTTP: it answers
`200 OK` when every resource is operational and `503 Service Unavailable`
otherwise, with a JSON body describing each resource's status:

```json
{"status": true, "resources": {"database": "HEALTHY", "http-server": "HEALTHY"}}
```

`HealthHandler` is an `httpserver.Handler` like any other: hand it to the
server, and it serves `GET /health` (`httpserver.DefaultHealthPath`). It needs
a List you hold, so build the List yourself and register it on `app.Run` as one
composite resource:

```go
resources := resource.NewList()
_ = resources.Register(databasePool)
_ = resources.Register(httpserver.NewServer(config, httpserver.NewHealthHandler(resources)))

err := app.Run(ctx,
	app.WithResource(resources), // one composite resource
	app.WithProcess(process.ListenToSignals),
)
```

The handler holds the List by pointer, so it also reports on resources
registered after it was built — including the HTTP server serving it.

To serve the health somewhere else use `WithPath`:

```go
health := httpserver.NewHealthHandler(resources).WithPath("/healthz")
```

`HealthHandler` is also a plain `net/http` handler (it has `ServeHTTP`), so you
can mount it on a router of your own.

The gRPC and the NATS servers have the very same handler, built from a List
the very same way: see [The health handler](#the-health-handler) and
[Exposing the health over NATS](#exposing-the-health-over-nats).

## The gRPC server

The `resource/server/grpc` package is the gRPC counterpart of the HTTP server.
`NewServer` builds a `Server` implementing `resource.Managed` from a `Config`
and its handlers:

```go
resources := resource.NewList()
_ = resources.Register(grpcserver.NewServer(config,
	grpcserver.NewHealthHandler(resources),
	&accountServiceHandler{},
))

err := app.Run(ctx, app.WithResource(resources))
```

`Init` registers the handlers, binds the listener, and serves in the
background. `Shutdown` stops the server gracefully, letting in-flight RPCs
complete, and falls back to an immediate stop when the context expires first.
The server is named
`grpc-server` (`grpcserver.DefaultName`) unless renamed with `WithName`.
Like the HTTP server it [starts once](#a-server-starts-once): an `Init` on a
running server, or on a server that was shut down, fails with
`grpcserver.ErrServerAlreadyStarted`, while a failed `Init` can be retried. To
serve again after a shutdown, build a new server.

### Configuration

`Config` carries the bind interface and the port, exactly like the
[HTTP configuration](#configuration). You can fill it in yourself:

```go
config := &grpcserver.Config{
	BindInterface: "127.0.0.1",
	BindPort:      50051,
}
```

Or read it from the environment with `NewConfigFromEnv`:

| Environment variable | Constant | Default |
| --- | --- | --- |
| `GRPC_BIND_INTERFACE` | `BindInterfaceEnvVar` | `0.0.0.0` (`DefaultBindInterface`) |
| `GRPC_BIND_PORT` | `BindPortEnvVar` | `50051` (`DefaultBindPort`) |

A `GRPC_BIND_PORT` that is not a number between 1 and 65535 fails with
`ErrInvalidBindPort`:

```go
config, err := grpcserver.NewConfigFromEnv()
if err != nil {
	return err // grpcserver.ErrInvalidBindPort
}
config.Address() // "0.0.0.0:50051"
```

`Init` fails with `ErrEmptyBindInterface` when the bind interface is empty, and
with `ErrBindPortNotInitialized` when the port is missing.

### Serving on your own listener

By default `Init` binds the address of the configuration. When you
already hold a listener — a Unix socket, an in-memory `bufconn` listener, or a
port you reserved yourself — hand it over with `WithListener`. The
configuration is then ignored, so it may stay empty:

```go
listener, err := net.Listen("tcp", "127.0.0.1:0") // the OS picks a free port
if err != nil {
	return err
}

server := grpcserver.NewServer(&grpcserver.Config{}, handlers...).WithListener(listener)
listener.Addr() // the address your clients dial
```

The server owns the listener from then on and closes it when it stops
serving. When the listener breaks, the status of the server becomes
`StatusFailed` and `Shutdown` returns the cause, wrapped in
`failed to serve gRPC`.

### Writing a handler

A gRPC handler registers its services on the `*grpc.Server`:

```go
type accountServiceHandler struct {
	accountv1.UnimplementedAccountServiceServer
}

func (h *accountServiceHandler) Register(server *grpc.Server) error {
	accountv1.RegisterAccountServiceServer(server, h)
	return nil
}
```

A handler that cannot register returns an error, which makes `Init` fail with
that error wrapped in `failed to register a gRPC handler`. The handlers are
registered before the listener is bound, so a failed registration leaves no
port behind.

### The health handler

`HealthHandler` reports the health of a `resource.List` over the standard
[gRPC health checking protocol](https://grpc.io/docs/guides/health-checking/).
Like its [HTTP counterpart](#exposing-the-health-of-your-resources) it is built
from a List you hold, and it reads the statuses of the List on every call:

```go
resources := resource.NewList()
_ = resources.Register(databasePool)
_ = resources.Register(grpcserver.NewServer(config, grpcserver.NewHealthHandler(resources)))
```

The protocol reports the health of named services, and the handler maps the
List onto them:

- The empty service name stands for the whole List: it is `SERVING` when every
  resource is operational, and `NOT_SERVING` otherwise.
- Every resource is a service named after it (`database`, `grpc-server`...):
  it is `SERVING` while the resource is healthy or degraded, and `NOT_SERVING`
  otherwise.
- Any other name is unknown: `Check` fails with `codes.NotFound`, and `Watch`
  streams `SERVICE_UNKNOWN`.

```go
client := grpc_health_v1.NewHealthClient(conn)

overall, err := client.Check(ctx, &grpc_health_v1.HealthCheckRequest{})
database, err := client.Check(ctx, &grpc_health_v1.HealthCheckRequest{Service: "database"})
```

`Watch` streams the current serving status of a service, then every change of
it, until the client leaves. It follows the resources that are in the List when
the stream opens.

### Responding with an error

`RespondWithError` is the gRPC counterpart of the HTTP error helper. It turns
an error into a gRPC status error carrying the `codes.Code` you give it and the
error's message. Return it from your service method:

```go
func (h *accountServiceHandler) GetAccount(ctx context.Context, req *accountv1.GetAccountRequest,
) (*accountv1.GetAccountResponse, error) {
	account, err := h.accounts.Find(ctx, req.GetId())
	if err != nil {
		return nil, grpcserver.RespondWithError(codes.NotFound, err)
	}
	return &accountv1.GetAccountResponse{Account: account}, nil
}
```

### Running the server standalone

`resource.Run` serves the handlers until the context is canceled or the server
fails, then stops gracefully:

```go
err := resource.Run(ctx, grpcserver.NewServer(config, grpcserver.NewHealthHandler(resources)))
```

## The NATS server

The `resource/server/nats` package serves NATS subjects the way the HTTP
server serves routes. A `Server` dispatches incoming messages to handler
functions, and it is a managed resource too.

### The NATS server as a managed resource

The server works on a plain `*nats.Conn`: connect with the official client,
and hand the connection to `NewServer` along with your handlers.

```go
nc, err := nats.Connect(nats.DefaultURL)
if err != nil {
	return err
}
defer nc.Close()

err = app.Run(ctx,
	app.WithResource(natsserver.NewServer(nc,
		orderHandler,
		shipmentHandler,
	)),
)
```

`Init` registers the handlers and subscribes them; `Shutdown` cancels every
subscription. A handler that fails to register makes `Init` fail and the server
report `StatusFailed`, and a nil connection fails initialization with
`ErrNilNATSConnection`. Like the other servers it
[starts once](#a-server-starts-once): calling `Init` (or `ListenAndServe`) on a
running server fails with `ErrServerAlreadyStarted` and leaves its
subscriptions untouched, and so does calling it on a server that was shut down,
which cannot be restarted. A start that failed leaves no subscription behind
and can be retried. To serve again after a shutdown, build a new server on the
same connection. The server
is named `nats-server` (`natsserver.DefaultName`) unless renamed with
`WithName`.

The connection stays yours: the server subscribes on it and publishes replies
through it, but never closes it. Share one connection between several servers,
or between a server and your own publishers, and close it once the application
has stopped. Closing it while the server runs makes `Shutdown` fail with
`nats.ErrConnectionClosed`, so do it after `app.Run` or `resource.Run` returns.

### Writing a handler

A NATS handler registers its subscriptions on the server with `HandleFunc`,
giving a subject, a queue group, and a `HandlerFunc`. The `HandlerFunc`
receives a `Request` — the NATS message and a context for its handling — and
replies through the `publish` function it is given:

```go
type OrderHandler struct{}

func (h *OrderHandler) Register(server *natsserver.Server) error {
	return server.HandleFunc("orders.created", "orders", h.handleCreated)
}

func (h *OrderHandler) handleCreated(r *natsserver.Request, publish natsserver.PublishFunc) error {
	// ... process r.Msg.Data within r.Context ...
	return natsserver.RespondWithJSON(publish, r.Msg, http.StatusOK, receipt)
}
```

An empty queue group delivers every message to every instance of your service;
a non-empty one makes NATS pick a single member of the group. Registering a
handler on an empty subject fails with `ErrEmptySubject`, and registering one
after the server started listening fails with `ErrServerAlreadyStarted`.

`r.Context` lives as long as the server: it is canceled when the server shuts
down (`Shutdown` or `Close`), so a handler doing long work can stop early. It
carries no deadline, so derive your own when a handler needs a timeout:

```go
func (h *OrderHandler) handleCreated(r *natsserver.Request, publish natsserver.PublishFunc) error {
	ctx, cancel := context.WithTimeout(r.Context, 5*time.Second)
	defer cancel()

	receipt, err := h.orders.Create(ctx, r.Msg.Data) // stops on timeout or server shutdown
	// ...
}
```

### Middlewares and error handling

A middleware is a `MiddlewareFunc`, a function wrapping a `HandlerFunc`.
Middlewares registered with `Use` wrap every handler, the first one being the
outermost. Register them before the server starts: the chain is assembled when
the server subscribes its handlers, and a later `Use` has no effect. The errors
handlers return are handed to the callback set with `OnError`, which — unlike
`Use` — you may call at any time, even while messages are being dispatched:

```go
server := natsserver.NewServer(nc, orderHandler)

server.Use(func(next natsserver.HandlerFunc) natsserver.HandlerFunc {
	return func(r *natsserver.Request, publish natsserver.PublishFunc) error {
		slog.InfoContext(r.Context, "message received", "subject", r.Msg.Subject)
		return next(r, publish)
	}
})

server.OnError(func(err error) {
	slog.Error("failed to handle a message", "err", err)
})
```

### Replying to requests

The response helpers follow the HTTP ones, publishing JSON bodies to the reply
subject of the request. NATS has no status codes of its own, so they name the
outcome with an HTTP status code too:

```go
natsserver.RespondWithJSON(publish, r.Msg, http.StatusOK, receipt)
natsserver.RespondWithError(publish, r.Msg, http.StatusBadRequest, err)
natsserver.RespondWithStatus(publish, r.Msg, http.StatusOK, "Success")
natsserver.RespondWithRawJSON(publish, r.Msg, http.StatusOK, []byte(`{"key":"value"}`))
```

`RespondWithError` and `RespondWithStatus` publish the same standard
`StatusResponse` body as the HTTP helpers (`code`, `status_text`, `message`),
which `natsserver.NewStatusResponse(code, message)` builds for you.
`RespondWithRawJSON` validates the data and trims the surrounding whitespace.
Every reply carries two headers: `Content-Type: application/json`, and `Status`
holding the text of the code (`OK`, `Not Found`...). As over HTTP, a body that
cannot be encoded, or raw data that is not valid JSON, is answered with a `500`
error body so the requester does not wait for its timeout, and fails with
`natsserver.ErrJSONEncoding`. Replying to a message that carries no reply
subject fails with `ErrNoReplySubject`.

### Exposing the health over NATS

`HealthHandler` reports the health of a `resource.List` to the requests
published on the `health` subject (`natsserver.DefaultHealthSubject`). It is
the counterpart of the [HTTP health handler](#exposing-the-health-of-your-resources):
built from a List you hold, it replies with the same JSON body, and with the
`OK` status when every resource is operational and `Service Unavailable`
otherwise:

```go
resources := resource.NewList()
_ = resources.Register(databasePool)
_ = resources.Register(natsserver.NewServer(nc, natsserver.NewHealthHandler(resources)))
```

```go
reply, err := nc.Request("health", nil, time.Second)
// reply.Header.Get("Status") == "OK"
// reply.Data == {"status":true,"resources":{"database":"HEALTHY","nats-server":"HEALTHY"}}
```

Several services usually share a NATS connection, so give each its own subject
with `WithSubject`:

```go
health := natsserver.NewHealthHandler(resources).WithSubject("orders.health")
```

### Running the server standalone

`resource.Run` initializes the server with its handlers, consumes messages until
the context is canceled or the server fails, and cancels every subscription on
the way out:

```go
err := resource.Run(ctx, natsserver.NewServer(nc, orderHandler))
```

You can also drive a `Server` by hand: register subscriptions with `HandleFunc`,
subscribe them without blocking with `ListenAndServe`, and cancel them with
`Close`.

## License

Licensed under MIT License.
