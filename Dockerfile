# --- build stage ---
FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/server ./cmd/server && \
    CGO_ENABLED=0 go build -o /out/migrate ./cmd/migrate && \
    CGO_ENABLED=0 go build -o /out/staffadmin ./cmd/staffadmin

# --- runtime stage ---
FROM alpine:3.20
RUN adduser -D -u 10001 appuser
COPY --from=build /out/server /usr/local/bin/server
COPY --from=build /out/migrate /usr/local/bin/migrate
COPY --from=build /out/staffadmin /usr/local/bin/staffadmin
COPY migrations /migrations
USER appuser
EXPOSE 8080
ENTRYPOINT ["server"]
