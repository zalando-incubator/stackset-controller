ARG BASE_IMAGE=registry.opensource.zalan.do/library/alpine-3.13:latest
FROM ${BASE_IMAGE}
LABEL maintainer="Team Teapot @ Zalando SE <team-teapot@zalando.de>"

ARG TARGETARCH

# add binary
ADD build/linux/${TARGETARCH}/stackset-controller /

ENTRYPOINT ["/stackset-controller"]
