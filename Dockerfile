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
# Defaults come from the environment, never from flags. The RS_* variables only
# supply each flag's default value, so a flag baked into CMD would win over them
# — a deployment asking for PostgreSQL via RS_DATABASE_DSN would silently keep
# writing to a container-local SQLite file and lose its data with every new
# container. As environment variables these stay overridable by the caller.
ENV RS_ADDR=":8080" \
    RS_DATABASE_DSN="sqlite:/data/spotlight.db"
CMD ["readme-spotlight"]
