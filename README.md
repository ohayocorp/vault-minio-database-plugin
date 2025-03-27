# Vault MinIO Database Plugin

This is a [Vault](https://www.vaultproject.io/) database plugin for managing [MinIO](https://min.io/) users.

## Functionality

This plugin allows Vault to dynamically create, update, and delete MinIO users. It can be used to:

*   Generate unique MinIO user credentials with specific policies.
*   Rotate MinIO user passwords.
*   Revoke MinIO user access.

## Usage

### Prerequisites

*   A running Vault server.
*   A running MinIO server.
*   Vault CLI configured to communicate with the Vault server.

### Configuration

1.  Build the plugin:

    ```sh
    make build
    ```

1.  Start Vault in dev mode (for testing):

    ```sh
    make start
    ```

1.  Start Minio (for testing):

    ```sh
    minio server /tmp/data
    ```

1.  Enable the database secrets engine:

    ```sh
    vault secrets enable database
    ```

1.  Configure the MinIO connection:

    ```sh
    vault write database/config/minio \
      plugin_name=vault-minio-database-plugin \
      allowed_roles="minio-role,test" \
      username="minioadmin" \
      password="minioadmin" \
      url="127.0.0.1:9000" \
      useSSL=false
    ```

1.  Define a role with associated creation statements:

    ```sh
    vault write database/roles/minio-role \
      db_name=minio \
      default_ttl="1h" \
      max_ttl="24h" \
        creation_statements='{"policy": "consoleAdmin"}'
    ```

    ```sh
    vault write database/roles/test \
      db_name=minio \
      default_ttl="1h" \
      max_ttl="24h" \
        creation_statements='{"policy": "readonly"}'
    ```

1.  Generate credentials:

    ```sh
    vault read database/creds/minio-role
    ```

See the [Makefile](Makefile) for example usage.