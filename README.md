# pubsub

Client for publishing and subscribing to events through the Platfor.me message broker.

## Client overview

`github.com/myplatforme/glider-client-go` is a lightweight Go client for module-to-module communication through the Platfor.me PubSub broker.

Use this client when your service needs to:
- publish business events (`order.created`, `invoice.paid`, etc.);
- subscribe to events from other modules;
- run request-response flows over events (`ack`);
- route messages by scope (`project`, `segment`, `root`);
- carry typed metadata and handler errors across service boundaries.

Core runtime behavior:
- outgoing requests are authenticated with HMAC-SHA256 using `Secret`;
- subscriptions are long-lived and stop when the provided `context.Context` is cancelled;
- publish calls support both fire-and-forget and waiting for a response;
- broker certificate mapping can be configured for private CA deployments.

## Connection

```go
import psclient "github.com/myplatforme/glider-client-go"

ps := psclient.New[any](psclient.Options[any]{
    Host:    "https://platfor.me", // PubSub config server address
    Secret:  "YOUR_SECRET",
    Module:  "module_name",
    Project: "project",            // optional, defaults to "root"
    Pubs: []string{                // events this module publishes
        "order.created",
        "user.req:project",        // ":project" — event is scoped to a project
    },
    Subs: []string{                // events this module subscribes to
        "order.updated",
        "user.res",
    },
})
```

## Event Flags

Flags are appended after the event name using `:` and are **only allowed in the `Pubs` list**. Using flags in `Subs` returns an `invalid subscriber format` error.

```go
Pubs: []string{
    "order.created",            // no flags — default routing
    "user.req:project",         // event scoped to a project
    "notify.all:segment:root",  // event delivered to both segment and root
    "payment.req:ack",          // acknowledgement expected (request–response)
},
Subs: []string{
    "order.updated",            // event name only — flags are not allowed
    "user.res",
},
```

### Available flags

| Flag | Description |
|---|---|
| `ack` | Publish with acknowledgement (request–response) |
| `project` | Event is routed in the context of a specific project |
| `segment` | Event is routed within the segment |
| `root` | Event is routed in the root context |
| `segment` + `root` | Event is delivered to both segment and root simultaneously |

> Flags `segment`, `project`, and `root` define the **delivery scope** of the message.  
> They can be combined: `"event.name:segment:root"`.


## Authentication

Every request is signed automatically using HMAC-SHA256 via the `Secret` option. No additional configuration is required.

Default validation settings:
- **Skew** — maximum clock drift between client and server: 30 s
- **TTL** — maximum request age: 60 s

To override:

```go
import "github.com/myplatforme/glider-client-go/authhmac"

ps := psclient.New[any](psclient.Options[any]{
    // ...
    AuthOptions: &authhmac.Options{
        Skew: 15 * time.Second,
        TTL:  30 * time.Second,
    },
})
```

## Certificates

By default brokers use the system certificate store. If a broker runs with a private CA, provide the path to the CA certificate.

**Option 1 — in code:**

```go
ps := psclient.New[any](psclient.Options[any]{
    // ...
    Certs: map[string]string{
        "broker-id-1": "/broker1/ca.pem",
        "broker-id-2": "/broker2/ca.pem",
    },
})
```

## Subscribing to events

The handler is called in a separate goroutine for each incoming message.  
The subscription is cancelled automatically when `ctx` is cancelled.

```go
ctx, cancel := context.WithCancel(context.Background())

ps.Sub(ctx, "order.updated", func(ctx context.Context, payload []byte) {
    // handle event
})

// cancel subscription
cancel()
```

## Publishing events

### Fire and forget

```go
err := ps.Pub(ctx, "order.created", payload).Do()
```

### With response

```go
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()

ctx, responsePayload, err := ps.Pub(ctx, "user.req", payload).Sub("user.res")
if err != nil {
    // err == context.DeadlineExceeded when the deadline is exceeded
}
```

### Publish in the context of a specific project

```go
ctx = psclient.WithProject(ctx, "project")
err := ps.Pub(ctx, "user.req", payload).Do()
```

## Typed message metadata

The client is parameterised with type `MD`, which must implement `proto.Message`. This type describes the metadata structure carried with each message.

### Initialise the client with a type

```go
import pb "your-service/proto"

ps := psclient.New[*pb.MyContext](psclient.Options[*pb.MyContext]{
    // ...
})
```

Using `any` disables typed metadata — `message.Context` will always return an error in that case.

### Set metadata when publishing

```go
import "github.com/myplatforme/glider-client-go/message"

ctx = message.WithContext(ctx, &pb.MyContext{
    UserId:   "u-123",
    Country:  "US",
    Language: "en",
})

err := ps.Pub(ctx, "user.req", payload).Do()
```

### Read metadata in a subscription handler

```go
ps.Sub(ctx, "user.req", func(ctx context.Context, payload []byte) {
    meta, err := message.Context[*pb.MyContext](ctx)
    if err != nil {
        // message.ErrMessageContextNotFound — metadata was not provided
        return
    }
    fmt.Println(meta.UserId, meta.Country)
})
```

Metadata is serialised automatically by the sender and deserialised by the receiver. If deserialisation fails the message is still delivered — `meta` simply will not be present in the context.

### Read metadata after receiving a response

```go
ctx, responsePayload, err := ps.Pub(ctx, "user.req", payload).Sub("user.res")
if err == nil {
    meta, err := message.Context[*pb.MyContext](ctx)
    // meta contains the metadata that the "user.res" handler put into the response
}
```

## Returning an error in a response

A subscription handler can return an error to the caller via the context. The error is stored in the `Error` field of the proto message and restored on the receiver side.

### Set an error (inside a subscription handler)

```go
ps.Sub(ctx, "user.req", func(ctx context.Context, payload []byte) {
    ctx = psclient.WithError(ctx, errors.New("user not found"))
    // continue processing with the error in context
})
```

### Read the error after receiving a response

```go
import "github.com/myplatforme/glider-client-go/metadata"

ctx, responsePayload, err := ps.Pub(ctx, "user.req", payload).Sub("user.res")
if err == nil {
    md := metadata.MetaFromContext(ctx)
    if md.Error != nil {
        fmt.Println("Handler error:", md.Error)
    }
}
```

## Routing metadata of an incoming message

```go
import "github.com/myplatforme/glider-client-go/metadata"

ps.Sub(ctx, "order.updated", func(ctx context.Context, payload []byte) {
    md := metadata.MetaFromContext(ctx)
    if md.Error != nil {
        fmt.Println("Error:", md.Error)
    }
    fmt.Println(md.Project, md.Segment, md.ID)
})
```

| Field | Description |
|---|---|
| `ID` | Unique message identifier |
| `Project` | Project in whose context the message was sent |
| `Segment` | Source segment |
| `Sub` | `true` if the message is a response to a previous request |
| `SubId` | ID of the original request |
| `Error` | Error set by the handler via `psclient.WithError` |

## Error handling

```go
import psclient "github.com/myplatforme/glider-client-go/client"

_, _, err := ps.Pub(ctx, "user.req", payload).Sub("user.res")
if errors.Is(err, psclient.ErrClientNotConnected) {
    // no broker is currently available
}
if errors.Is(err, context.DeadlineExceeded) {
    // response did not arrive within the context deadline
}
```
