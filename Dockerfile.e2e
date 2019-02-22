FROM registry.opensource.zalan.do/stups/alpine:latest
MAINTAINER Team Teapot @ Zalando SE <team-teapot@zalando.de>

# add binary
ADD build/linux/e2e /

ENTRYPOINT ["/e2e", "-test.v", "-test.parallel", "64"]
