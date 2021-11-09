# MQ Go

A minimalist toy message queuing server that operates over HTTP. Messages are distributed to any clients registered to receive them. No channels, topics, etc are supported.

## Usage

1. Run `go build .`
2. Run `mq-go`. By default, logging goes to `stderr`, the server runs on port 8080, a buffer of 5 messages can wait to be distributed at once, and 3 worker threads are spawned.

Flags are:

```
Usage of ./mq-go:
  -jobs int
    number of buffered send jobs (default 5)
  -port int
    listening port (default 8080)
  -workers int
    number of send worker threads (default 3)
```

## Questions?

*Why HTTP?*

Because I knew a little bit about how `net/http` works and TCP protocols seemed like too much work for a quick toy.

*Why not use one of the many existing solutions?*

Because I was curious and wanted something very simple to experiment with containerization and orchestration.

*You wrote this because you're messing with Kubernetes?*

Yes, and I didn't also want to add "messing with RabbitMQ" to that.

*Can or should I use this software for anything meaningful?*

Probably not. As it stands anyone who is able to find the server can send a message. There's minimal rate limiting. There is no authentication. Also, I am not a Go expert so there's no guarantee the code isn't full of worst practices, race conditions, and/or straight-up broken bits.

## Client Requests

1) CONNECT: An HTTP POST request to `/connect`. Establishes as connection. The body should be a JSON document like

```json
{
    "ClientURL": "svc.domain.tld/mqg",
    "mode": "send",
}
```

The `'mode'` key should have one of two values: `'send'` or `'receive'`. A `'send'` value will register the client as a message-submitter. A `'receive'` value will register the client to receive messages.

Server Response: `200 OK` with body like
```json
{
    "id": 123456,
}
```

2) SEND: An HTTP POST request to `/send`. The body of the request should include the message, which is a string, like:

```json
{
    "id": 12346,
    "body": "Here is the message body.",
 }
```

Server Response: `200 OK` with body like
```json
{
    "status": "Message enqueued.",
    "id": 123456
}
```

The root `'id'`key should contain a value that matched a connected sender client.

3) DISCONNECT: An HTTP Post request to `/disconnect`. The JSON body needs to include the ID of the connection.

```json
{
    "id": 123456
}
```

Server Response: `200 OK` with body like
```json
{
    "id": 123456,
    "message": "Disconnected."
}
```

## Server Requests

1) MESSAGE: An HTTP POST to a client's `ClientURL` with a body like
```json
{
    "body": "Here is the message body."
}
```
