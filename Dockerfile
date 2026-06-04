# syntax=docker/dockerfile:1.7

FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS build
WORKDIR /src
ARG TARGETOS
ARG TARGETARCH
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download
COPY . .
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags="-s -w" -o /out/dawnlink ./cmd/dawnlink
RUN mkdir /data

FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=build /out/dawnlink /app/dawnlink
COPY --from=build --chown=65532:65532 /data /data
ENV PORT=8080 \
    URL=http://localhost:8080/ \
    DEFAULT_LOCALE=en \
    DATABASE_FILE=/data/db.sqlite
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/app/dawnlink"]
