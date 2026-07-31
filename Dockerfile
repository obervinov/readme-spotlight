# Build a static binary (modernc SQLite is pure Go, so CGO stays off).
FROM golang:1.25 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/readme-spotlight ./cmd/readme-spotlight

# Alpine (not distroless) so the image ships a shell/coreutils — the shared
# release pipeline validates each arch with `docker run <image> uname -m`, which
# needs a runnable command in the image. CMD (not ENTRYPOINT) keeps that check
# working: `docker run <image> uname -m` overrides it, `docker run <image>`
# starts the server.
FROM alpine:3.20
RUN adduser -D -u 65532 app && mkdir -p /data && chown app /data
COPY --from=build /out/readme-spotlight /usr/local/bin/readme-spotlight
USER app
EXPOSE 8080
VOLUME /data
CMD ["readme-spotlight", "--addr", ":8080", "--db", "sqlite:/data/spotlight.db"]
