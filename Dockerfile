FROM registry.opensource.zalan.do/stups/alpine:latest
MAINTAINER Team Teapot @ Zalando SE <team-teapot@zalando.de>

# add binary
ADD build/linux/stackset-controller /

ENTRYPOINT ["/stackset-controller"]
