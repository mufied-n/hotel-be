# --- build stage ---
FROM golang:1.27-alpine AS build
WORKDIR /src

# Cache dependencies
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

# Build with BuildKit compiler cache mounts, stripping symbols and trimming paths
COPY . .
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -ldflags="-s -w" -trimpath -o /out/server ./cmd/server && \
    CGO_ENABLED=0 go build -ldflags="-s -w" -trimpath -o /out/migrate ./cmd/migrate && \
    CGO_ENABLED=0 go build -ldflags="-s -w" -trimpath -o /out/staffadmin ./cmd/staffadmin

# --- runtime stage ---
FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata && \
    adduser -D -u 10001 appuser

COPY --from=build /out/server /usr/local/bin/server
COPY --from=build /out/migrate /usr/local/bin/migrate
COPY --from=build /out/staffadmin /usr/local/bin/staffadmin
COPY migrations /migrations

USER appuser
EXPOSE 8080
ENTRYPOINT ["server"]
