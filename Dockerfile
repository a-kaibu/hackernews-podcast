FROM golang:1.26-bookworm AS build

WORKDIR /app

RUN --mount=type=cache,target=/go/pkg/mod/,sharing=locked \
    --mount=type=bind,source=go.sum,target=go.sum \
    --mount=type=bind,source=go.mod,target=go.mod \
    go mod download -x

RUN --mount=type=cache,target=/go/pkg/mod/ \
    --mount=type=bind,target=. \
    CGO_ENABLED=0 GOARCH=$TARGETARCH go build -ldflags="-s" -trimpath -o /bin/hackernews-podcast .

FROM gcr.io/distroless/static-debian13

WORKDIR /app
COPY --from=build /bin/hackernews-podcast /app/hackernews-podcast

ENTRYPOINT ["/app/hackernews-podcast"]
