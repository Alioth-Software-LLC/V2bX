# Build go
FROM golang:1.25.0-alpine AS builder
WORKDIR /app
COPY . .
ENV CGO_ENABLED=0
RUN GOEXPERIMENT=jsonv2 go mod download
RUN GOEXPERIMENT=jsonv2 go build -v -o V2bX -tags "sing xray hysteria2 with_quic with_grpc with_utls with_wireguard with_acme with_gvisor"

# Release
FROM  alpine
ARG V2BX_IMAGE_VERSION="v0.4.1-alioth.1"
ARG V2BX_IMAGE_REVISION="unknown"
ARG V2BX_UPSTREAM_REVISION="71277de69efbbc86c23ad8ae02b68efd174e5756"
ARG V2BX_PATCH_REVISION="3a7b882caf935d68d4fe886125cbed06a6e2d020"
LABEL org.opencontainers.image.source="https://github.com/Alioth-Software-LLC/V2bX" \
      org.opencontainers.image.version="${V2BX_IMAGE_VERSION}" \
      org.opencontainers.image.revision="${V2BX_IMAGE_REVISION}" \
      com.alioth.v2bx.upstream-revision="${V2BX_UPSTREAM_REVISION}" \
      com.alioth.v2bx.patch-revision="${V2BX_PATCH_REVISION}" \
      com.alioth.v2bx.patch="empty-user-list-revokes-authorized-users"
# 安装必要的工具包
RUN  apk --update --no-cache add tzdata ca-certificates \
    && cp /usr/share/zoneinfo/Asia/Shanghai /etc/localtime
RUN mkdir /etc/V2bX/
COPY --from=builder /app/V2bX /usr/local/bin

ENTRYPOINT [ "V2bX", "server", "--config", "/etc/V2bX/config.json"]
