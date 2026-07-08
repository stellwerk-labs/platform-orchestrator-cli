FROM --platform=$BUILDPLATFORM golang:1.25.1-alpine AS builder

ARG TARGETOS
ARG TARGETARCH

WORKDIR /app

# https://stackoverflow.com/questions/36279253/go-compiled-binary-wont-run-in-an-alpine-docker-container-on-ubuntu-host
ENV CGO_ENABLED=0 GOOS=linux GOWORK=off

RUN apk add git

COPY . .

RUN --mount=target=. \
    --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -o /octl ./

FROM gcr.io/distroless/static:nonroot AS final
COPY --chown=nonroot:nonroot --from=builder /octl /octl
ENTRYPOINT ["/octl"]
