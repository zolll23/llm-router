FROM golang:1.26-bookworm AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/router ./cmd/router

# Root user: named volume /data is root-owned; local compose needs write access.
FROM gcr.io/distroless/static-debian12
WORKDIR /
COPY --from=build /out/router /router
COPY configs/config.docker.yaml /configs/config.yaml
EXPOSE 8080
ENTRYPOINT ["/router"]
CMD ["-config", "/configs/config.yaml"]
