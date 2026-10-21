# GoReleaser doesn't build go files, it's just copying already built binary:
# https://goreleaser.com/customization/docker/#the-docker-build-context
FROM gcr.io/distroless/static:nonroot@sha256:e2e927ec666bae08560abb3c55d0659eceabb657f56b6782ab500a9fc7f555e3

ARG TARGETARCH
COPY linux/${TARGETARCH}/octl /usr/local/bin/octl
COPY licenses /licenses

ENTRYPOINT ["/usr/local/bin/octl"]
