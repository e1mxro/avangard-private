# syntax=docker/dockerfile:1.7

# Multi-stage build for AVANGARD server + client. Resulting images are
# distroless-style (~25 MB) without a shell.
FROM golang:1.25-alpine AS build
RUN apk add --no-cache git
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/avangard-server ./cmd/avangard-server && \
    CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/avangard-client ./cmd/avangard-client

FROM gcr.io/distroless/static-debian12:nonroot AS server
COPY --from=build /out/avangard-server /usr/local/bin/avangard-server
USER nonroot
EXPOSE 443/tcp 443/udp
ENTRYPOINT ["/usr/local/bin/avangard-server"]
CMD ["run", "--config", "/etc/avangard/server.yaml"]

FROM gcr.io/distroless/static-debian12:nonroot AS client
COPY --from=build /out/avangard-client /usr/local/bin/avangard-client
USER nonroot
ENTRYPOINT ["/usr/local/bin/avangard-client"]
