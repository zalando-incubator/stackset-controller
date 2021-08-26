FROM registry.opensource.zalan.do/library/alpine-3.13:latest
LABEL maintainer="Team Teapot @ Zalando SE <team-teapot@zalando.de>"

# add binary
ADD build/linux/stackset-controller /

ENTRYPOINT ["/stackset-controller"]
