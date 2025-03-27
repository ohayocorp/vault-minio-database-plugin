GOARCH = amd64

UNAME = $(shell uname -s)

ifndef OS
	ifeq ($(UNAME), Linux)
		OS = linux
	else ifeq ($(UNAME), Darwin)
		OS = darwin
	endif
endif

.DEFAULT_GOAL := all

all: fmt build start

build:
	GOOS=$(OS) GOARCH="$(GOARCH)" CGO_ENABLED=0 go build -o vault/plugins/vault-minio-database-plugin cmd/vault-minio-database-plugin/main.go

start:
	vault server -dev -dev-root-token-id=root -dev-plugin-dir=./vault/plugins -log-level trace

minio:
	mkdir -p /tmp/minio-test
	minio server /tmp/minio-test

enable:
	vault secrets enable database

	vault write database/config/minio \
      plugin_name=vault-minio-database-plugin \
      allowed_roles="minio-role,test" \
      username="minioadmin" \
      password="minioadmin" \
      url="127.0.0.1:9000" \
      useSSL=false

	vault write database/roles/minio-role \
      db_name=minio \
      default_ttl="1h" \
      max_ttl="24h" \
	  creation_statements='{"policy": "consoleAdmin"}'

	vault write database/roles/test \
      db_name=minio \
      default_ttl="1h" \
      max_ttl="24h" \
	  creation_statements='{"policy": "readonly"}'

	creds=$$(vault read -format=json database/creds/minio-role); \
	username=$$(echo "$$creds" | jq -r ".data.username"); \
	password=$$(echo "$$creds" | jq -r ".data.password"); \
	vault write database/config/minio-vault \
      plugin_name=vault-minio-database-plugin \
      allowed_roles="minio-role" \
      username="$$username" \
      password="$$password" \
      url="127.0.0.1:9000" \
      useSSL=false

	vault write -force database/rotate-root/minio-vault
	vault write -force database/rotate-root/minio-vault

clean:
	rm -f ./vault/plugins/vault-minio-database-plugin

fmt:
	go fmt $$(go list ./...)

.PHONY: build clean fmt start enable
