# syntax=docker/dockerfile:1
# flowcli — the FlowControl TUI, built and run in a container so a host that
# cannot (or would rather not) run the Go toolchain natively can still launch it.
# Multi-stage: a throwaway builder compiles a static binary that runs on a tiny
# final image. The TUI only needs a tty, which the container runtime provides.
#
# Layer strategy (mirroring flowd/Dockerfile):
#   * go.mod/go.sum copied and `go mod download` first, so module downloads live
#     in their own layer and source edits never re-download.
#   * BuildKit cache mounts (--mount=type=cache) persist GOMODCACHE and the Go
#     build cache across rebuilds, so a source change recompiles only the
#     changed packages instead of everything.
#   * A final FROM swaps in just the static binary -> a small runtime image.

FROM golang:1.25 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go mod download
COPY . .
# CGO_ENABLED=0 -> a single static binary; the TUI is pure Go / termios.
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/flowcli .

FROM alpine:3.20
COPY --from=build /out/flowcli /usr/local/bin/flowcli
ENTRYPOINT ["flowcli"]
