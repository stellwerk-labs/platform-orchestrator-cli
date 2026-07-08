# GoReleaser doesn't build go files, it's just copying already built binary:
# https://goreleaser.com/customization/docker/#the-docker-build-context
FROM gcr.io/distroless/static:nonroot

COPY octl /usr/local/bin/octl

ENTRYPOINT ["/usr/local/bin/octl"]
