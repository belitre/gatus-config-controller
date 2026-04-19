FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=canary
ARG COMMIT=unknown
ARG DATE=unknown
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags "-X main.Version=${VERSION} -X main.Commit=${COMMIT} -X main.Date=${DATE}" \
    -o gatus-config-controller ./cmd/

FROM gcr.io/distroless/static:nonroot
COPY --from=builder /app/gatus-config-controller /gatus-config-controller
ENTRYPOINT ["/gatus-config-controller"]
