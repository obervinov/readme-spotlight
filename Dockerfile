# Build a static binary (modernc SQLite is pure Go, so CGO stays off).
FROM golang:1.25 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/readme-spotlight ./cmd/readme-spotlight

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/readme-spotlight /readme-spotlight
EXPOSE 8080
VOLUME /data
ENTRYPOINT ["/readme-spotlight"]
CMD ["--addr", ":8080", "--db", "sqlite:/data/spotlight.db"]
