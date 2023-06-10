FROM --platform=${BUILDPLATFORM} golang:1.20 AS builder
ARG TARGETOS
ARG TARGETARCH

WORKDIR /app
ENV CGO_ENABLED=0
ENV GOOS=${TARGETOS}
ENV GOARCH=${TARGETARCH}

COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o switchbot-mackerel .

FROM gcr.io/distroless/static:nonroot AS runner
WORKDIR /app
COPY --from=builder --chown=nonroot:nonroot /app/switchbot-mackerel /app/switchbot-mackerel
ENTRYPOINT ["/app/switchbot-mackerel"]
