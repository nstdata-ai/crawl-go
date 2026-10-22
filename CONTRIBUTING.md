# Contributing

Keep the public import path and existing methods compatible where possible. Record behavior changes in CHANGELOG.md. Verify routes, authentication, request fields and results against the public backend implementation when available, then official API documentation. Do not add automatic task-creation retries.

## Checks

Use Go 1.20.14 for the compatibility floor and a maintained Go release for production checks:

```sh
gofmt -w *.go example/*.go
go vet ./...
go test -race ./...
go build ./...
```

Tests use synthetic fixtures and loopback HTTP servers, without credentials or paid API calls. Cover changes through public client/results and the executable example: failures, absent/empty content, references, context, response limits and resource cleanup. README Go examples are compiled in isolated modules.

## Local module consumption

Before the version is published, run these commands from a separate application directory, replacing the last argument with your checkout's path:

```sh
go mod init example.com/crawl-consumer
go mod edit -require=github.com/nstdata-ai/crawl-go@v1.0.0
go mod edit -replace=github.com/nstdata-ai/crawl-go=/path/to/crawl-go
```

Import `github.com/nstdata-ai/crawl-go` in your application and run `go build ./...`. This replacement is local development configuration; remove it before consuming a published module. It does not verify public availability.

## Service examples

From this checkout, set `NSTDATA_API_TOKEN` locally and run only the command you intend:

```sh
go run ./example scrape https://example.com/
go run ./example submit-scrape https://example.com/
go run ./example resume-scrape SAVED_TASK_ID
go run ./example submit-crawl https://your-authorized-site.example/
go run ./example resume-crawl SAVED_CRAWL_ID
```

These are independent alternatives. Submission can incur charges. Store the returned ID before continuing; do not resubmit after an uncertain outcome. The example caps status queries at 20, page batches at 100 and total waiting at two minutes. Site submission sets maxPages=2/maxDepth=1; native discovery can return fewer pages. Keep IDs, references, private content and local credentials out of published fixtures.

Before a release, confirm license rights, version/tag, corresponding CI, artifact contents, maintained-runtime tests and representative service acceptance. After authorized publication, verify installation from the public module source without local replacement. Local checks do not establish release or deployed-service status.
