FROM golang:1.24 AS build

COPY . /src
WORKDIR /src
RUN make build

FROM busybox:1.37.0

COPY --from=build /src/vault/plugins/vault-minio-database-plugin /opt/vault-minio-database-plugin