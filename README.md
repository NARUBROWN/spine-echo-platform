English | [Korean](README.ko.md)

# spine-echo-platform

An Echo v5.4.4 platform and convenience constructors for Spine HTTP v3. The Go package name is `platform`; Go 1.25.5 or later is required.

## Preview installation

Use this channel to try features under development and check compatibility. The published tag is [`v0.2.0-preview.5`](https://github.com/NARUBROWN/spine-echo-platform/tree/v0.2.0-preview.5). Its `go.mod` specifies Spine `v0.7.0-preview.4` and Go 1.25.5 or later.

```sh
go get github.com/NARUBROWN/spine-echo-platform@v0.2.0-preview.5
```

See the [migration guide](docs/migration/v0.2.1.md) for the verification scope of the next version, `v0.2.1`.

Other channels: [RC](https://github.com/NARUBROWN/spine-echo-platform/tree/rc) | [Stable](https://github.com/NARUBROWN/spine-echo-platform/tree/main).

This module depends on Spine and Echo and provides an EchoAdapter implementing Spine's public `contract/http` contract. Contracts are included in the Spine module; no separate contract module or repository is needed.

## Start with Spine CLI

Generate a project with the CLI, then check `go.mod` for the selected platform version. To select a particular tag in an existing application, use the direct installation commands below.

Since Spine `v0.6.0`, the official way to create a project is Spine CLI's `new` command. Install the [latest public CLI](https://github.com/NARUBROWN/spine-cli#quick-start). The embedded verification recipe selects a compatible combination of Spine, the Echo platform, and Echo.

```sh
go install github.com/NARUBROWN/spine-cli/cmd/spine@v0.4.4-preview.2
spine version
spine new todo-api --module github.com/acme/todo-api --channel preview
cd todo-api
spine dev
```

In another terminal, run `curl http://localhost:8080/health` and check for `200 OK` with `{"status":"ok"}`. See the generated `go.mod` for selected dependency versions.

By default, `new` wires the platform, installs dependencies, checks static contracts, and builds the project. Start the server with `spine dev`. If you used `--skip-install` to generate files only, run `go mod tidy` separately.

## Connect the platform to an existing application

Add the platform dependency to your existing Go module.

```sh
go get github.com/NARUBROWN/spine-echo-platform@v0.2.0-preview.5
```

```go
import (
    "log"

    "github.com/NARUBROWN/spine-echo-platform"
    "github.com/NARUBROWN/spine/pkg/boot"
)

func main() {
    app := platform.New()
    if err := app.Run(boot.Options{Address: ":8080", HTTP: &boot.HTTPOptions{}}); err != nil {
        log.Fatal(err)
    }
}
```

`platform.New()` is equivalent to `spine.New(platform.Option())`. For manual composition, use `spine.WithHTTPAdapter(factory)`. The kernel's `spine.New()` does not select an HTTP platform by default.

NewFactory provides `github.com/NARUBROWN/spine/contract/http.Factory`. Preparation mounts the fallback, WebSocket GET routes, and native hooks in that order. The platform handles raw request input, response output, and HTTP server shutdown. The kernel handles DTO binding, multipart parsing, response staging, and the WebSocket message engine.

`WithEcho` is a typed platform option receiving `*echo.Echo` from `github.com/labstack/echo/v5`. Native routes bypass Spine's HTTP interceptors, body limits, and staging. The native store and Spine request store are separate; use the standard context for shared request values.

## TLS

In the current v3 development source, configure the managed server's TLS with `boot.HTTPOptions.TLS`. Load certificates with `crypto/tls` first.

```go
certificate, err := tls.LoadX509KeyPair("server.crt", "server.key")
if err != nil {
    return err
}
return app.RunContext(ctx, boot.Options{
    Address: ":8443",
    HTTP: &boot.HTTPOptions{
        TLS: &tls.Config{Certificates: []tls.Certificate{certificate}},
    },
})
```

A nil TLS configuration uses plaintext HTTP. Without an explicit address, TLS uses `:https` and plaintext uses `:http`. Prepare checks TLS support and static certificates before native hooks run. Certificate selection callbacks run during actual connections. Configuration and certificate bytes are copied; callers must ensure private key objects and callbacks support concurrent use.

The prepared adapter's `Serve(listener)` uses the same configuration. Passing `httpcontract.WithTLS(config)` overrides the default at serving time; the last option wins. `httpcontract` is an import alias for `github.com/NARUBROWN/spine/contract/http`. `WithTLS(nil)` and nil options return errors; there is no additional public `ServeTLS`. When attaching only a handler to an external server, that server owns TLS. HTTP version selection and ALPN policy are outside this change.

## Native Echo v5 extensions

v0.1.0 uses Spine's HTTP adapter v2 contract. Constructor and controller registration APIs remain unchanged. Native Echo extensions use the v5 import and pointer Context.

```go
import "github.com/labstack/echo/v5"

app := platform.New(platform.WithEcho(func(e *echo.Echo) {
    e.GET("/native", func(ctx *echo.Context) error {
        return ctx.String(200, "native")
    })
}))
```

- Handlers and middleware receive `*echo.Context`.
- `HTTPErrorHandler` has the argument order `func(ctx *echo.Context, err error)`.
- Logging uses the standard `log/slog` package.
- Read route arguments with `PathValues()`. `Response()` returns `http.ResponseWriter`; use `echo.UnwrapResponse` for Echo response state. Native middleware response wrappers must also provide `Unwrap() http.ResponseWriter`.

`NewFactory(options...)`, `Option(options...)`, and `New(options...)` accept the same Echo options. Registration and capability checks do not execute these functions. Each preparation runs them once, in order, after registering fallback and WebSocket routes. Nil functions are rejected during preparation.

Response `WriteJSON` accepts only a `json.RawMessage` serialized by the core. It retains Echo's JSON output path, including newlines, pretty printing, and custom JSONSerializer behavior. Custom serializers receive RawMessage instead of the original DTO and must support that type. The v1 `Config.NativeHooks` and `App.Transport` APIs are not used.

The platform preserves input error status and causes through Echo v5's HTTP status code interface. WebSocket upgrades receive the original HTTP writer. An `http.Server` owned by the platform handles serving and shutdown.

## HTTP v3 input binding

Host lookup reads Go's `Request.Host` separately. `Header("Host")` and `Headers()["Host"]` expose the same request value; case variants of Host behave identically. The port is preserved, and `X-Forwarded-Host` does not replace it. Modifying the returned map does not modify the original request headers.

The required v3 `RawQuery` passes `Request.URL.RawQuery` to the core without decoding. Core query slices support repeated keys and comma separation. `tags=a,b` binds two elements; `tags=a%2Cb` binds one element containing a comma. In `tags=a,%20b`, the second element retains its leading space. The core also URL-decodes ordinary Query and Queries lookups.

The default JSON decoder uses Spine's `pkg/jsonx.Unmarshal`. Explicit null for `int`, `bool`, `string`, ordinary structs, and arrays is rejected with a 400 input error, including nested fields and collection elements. Missing fields and null pointers, maps, and slices are preserved; the validator decides whether they are required. Custom `json.Unmarshaler` implementations retain their own null handling.

Spine DTO input does not call Echo Binder or JSONSerializer. Replacing Binder through `WithEcho` affects native Echo routes; response JSONSerializer remains part of Echo's output path. Request size limits and HTTP error conversion remain in effect.

These examples describe `v0.2.0`, which uses HTTP v3. Existing `v0.1.0` users should read the [HTTP v3 migration guide](https://github.com/NARUBROWN/spine/blob/v0.7.1/docs/migration/http-v3.md).

## Prepare without opening a listener

The `spine.App` returned by `platform.New()` exposes `app.Prepare(ctx, options)`. With HTTP options configured, attach the handler returned by `spine.NetHTTPHandler(prepared)` to an external HTTP server or `httptest`. Preparation does not open listeners or start consumers and custom transports. Depending on configuration, broker readiness checks and custom Init may run.

Callers release prepared resources with `Close(ctx)`. Once Close starts, new requests receive 503. Framework resources are cleaned up after in-flight requests finish. Cleanup continues after a wait timeout; call Close again to wait for completion. The caller shuts down an external server's listener.

See the [Spine v0.6.0 migration guide](https://github.com/NARUBROWN/spine/blob/v0.6.0/docs/migration/http-adapter-modules.md#%EB%A6%AC%EC%8A%A4%EB%84%88-%EC%97%86%EC%9D%B4-%EC%95%B1-%EC%A4%80%EB%B9%84%ED%95%98%EA%B8%B0) for API and resource ownership.

## Development repository layout

```text
VscodeProjects/
  spine/
  spine-echo-platform/
```

The current source `go.mod` uses Spine `v0.7.0-preview.4` without local replacements. Source CI uses a pinned core commit. Check the release manifest for a public tag's verification status. Follow [release operations](RELEASING.md) for checks before and after publication. To develop alongside the kernel in the layout above, create a local workspace with `go work init . ../spine`, or point a replacement in an existing go.work to a feature worktree. Do not commit the local go.work.

v0.1.0 uses Spine `v0.6.0`. Install and build pinned public versions without go.work or local replacements.

See the [related issue](https://github.com/NARUBROWN/spine/issues/9) and [minimal E2E example](https://github.com/NARUBROWN/spine/tree/v0.6.0/cmd/demo).

```sh
GOWORK=off go mod verify
GOWORK=off go test -race ./... -count=1
```

Public CI checks a pinned public Spine version without go.work or local replacements.

## Releases and public contracts

This platform publishes its own versions and release channels. The Spine version in `go.mod` is the minimum dependency for its public contracts and is managed separately from the core release channel. CI and public installation checks verify the platform against this module's dependencies. Core releases verify combinations of a new core and existing platform tags.
