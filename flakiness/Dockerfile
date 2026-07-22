FROM golang:1.25 AS builder

WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /flakiness-tool ./cmd/flakiness-tool

FROM gcr.io/distroless/static:nonroot

COPY --from=builder /flakiness-tool /usr/local/bin/flakiness-tool

USER nonroot:nonroot
ENTRYPOINT ["flakiness-tool"]
