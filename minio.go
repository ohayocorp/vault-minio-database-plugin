package vaultminio

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/hashicorp/go-secure-stdlib/strutil"
	dbplugin "github.com/hashicorp/vault/sdk/database/dbplugin/v5"
	"github.com/hashicorp/vault/sdk/database/helper/dbutil"
	"github.com/hashicorp/vault/sdk/helper/template"
)

// Minio implements dbplugin's Database interface.
type Minio struct {
	// This protects the config from races while also allowing multiple threads
	// to read the config simultaneously when it's not changing.
	mux sync.RWMutex

	// The root credential config.
	config map[string]interface{}

	usernameProducer template.StringTemplate
}

type ClientRoles struct {
	ClientID string   `json:"client_id"`
	Roles    []string `json:"roles"`
}

type CreationStatement struct {
	Policy string `json:"policy"`
}

const (
	defaultUserNameTemplate = `{{ printf "v-%s-%s-%s-%s" (.DisplayName | truncate 15) (.RoleName | truncate 15) (random 20) (unix_time) | truncate 100 }}`
)

var _ dbplugin.Database = (*Minio)(nil)

// New returns a new Minio instance
func New() (interface{}, error) {
	db := &Minio{}
	return dbplugin.NewDatabaseErrorSanitizerMiddleware(db, db.SecretValues), nil
}

// Type returns the TypeName for this backend
func (minio *Minio) Type() (string, error) {
	return "minio", nil
}

// Initialize is called on `$ vault write database/config/:db-name`,
// or when you do a creds call after Vault's been restarted.
func (minio *Minio) Initialize(ctx context.Context, req dbplugin.InitializeRequest) (dbplugin.InitializeResponse, error) {
	usernameTemplate, err := strutil.GetString(req.Config, "username_template")
	if err != nil {
		return dbplugin.InitializeResponse{}, fmt.Errorf("failed to retrieve username_template: %w", err)
	}
	if usernameTemplate == "" {
		usernameTemplate = defaultUserNameTemplate
	}

	up, err := template.NewTemplate(template.Template(usernameTemplate))
	if err != nil {
		return dbplugin.InitializeResponse{}, fmt.Errorf("unable to initialize username template: %w", err)
	}

	_, err = up.Generate(dbplugin.UsernameMetadata{})
	if err != nil {
		return dbplugin.InitializeResponse{}, fmt.Errorf("invalid username template: %w", err)
	}

	// Validate the config to provide immediate feedback to the user.
	// Ensure required string fields are provided in the expected format.
	for _, requiredField := range []string{"username", "password", "url", "useSSL"} {
		raw, ok := req.Config[requiredField]
		if !ok {
			return dbplugin.InitializeResponse{}, fmt.Errorf(`%q must be provided`, requiredField)
		}
		if _, ok := raw.(string); !ok {
			return dbplugin.InitializeResponse{}, fmt.Errorf(`%q must be a string`, requiredField)
		}
	}

	// Optionally, test the given config to see if we can make a successful call.
	if req.VerifyConnection {
		client, err := BuildClient(ctx, req.Config)

		if err != nil {
			return dbplugin.InitializeResponse{}, fmt.Errorf("client test of login failed: %w", err)
		}

		_, err = client.Client.ServerInfo(ctx)

		if err != nil {
			return dbplugin.InitializeResponse{}, fmt.Errorf("client test of login failed: %w", err)
		}
	}

	// Everything's working, write the new config to memory and storage.
	minio.mux.Lock()
	defer minio.mux.Unlock()

	minio.usernameProducer = up
	minio.config = req.Config

	response := dbplugin.InitializeResponse{
		Config: req.Config,
	}

	return response, nil
}

// NewUser is called on `$ vault read database/creds/:role-name`
// and it's the first time anything is touched from `$ vault write database/roles/:role-name`.
// This is likely to be the highest-throughput method for this plugin.
func (minio *Minio) NewUser(ctx context.Context, req dbplugin.NewUserRequest) (dbplugin.NewUserResponse, error) {
	username, err := minio.usernameProducer.Generate(req.UsernameConfig)
	if err != nil {
		return dbplugin.NewUserResponse{}, err
	}

	stmt, err := newCreationStatement(req.Statements)
	if err != nil {
		return dbplugin.NewUserResponse{}, fmt.Errorf("unable to read creation_statements: %w", err)
	}

	// Don't let anyone write the config while we're using it for our current client.
	minio.mux.RLock()
	defer minio.mux.RUnlock()

	client, err := BuildClient(ctx, minio.config)
	if err != nil {
		return dbplugin.NewUserResponse{}, fmt.Errorf("can't create madmin client: %w", err)
	}

	if err := client.CreateUser(ctx, username, req.Password, stmt, req.Expiration); err != nil {
		return dbplugin.NewUserResponse{}, fmt.Errorf("unable to create user %s, %w", username, err)
	}

	resp := dbplugin.NewUserResponse{
		Username: username,
	}

	return resp, nil
}

// Rotate user password
func (minio *Minio) UpdateUser(ctx context.Context, req dbplugin.UpdateUserRequest) (dbplugin.UpdateUserResponse, error) {
	// Don't let anyone write the config while we're using it for our current client.
	minio.mux.RLock()
	defer minio.mux.RUnlock()

	client, err := BuildClient(ctx, minio.config)
	if err != nil {
		return dbplugin.UpdateUserResponse{}, fmt.Errorf("can't create madmin client: %w", err)
	}

	if req.Password != nil {
		if err := client.UpdateUser(ctx, req.Username, req.Password.NewPassword); err != nil {
			return dbplugin.UpdateUserResponse{}, fmt.Errorf("unable to change password for user %s, %w", req.Username, err)
		}
	}

	return dbplugin.UpdateUserResponse{}, nil
}

// DeleteUser is used to delete users from Minio
func (minio *Minio) DeleteUser(ctx context.Context, req dbplugin.DeleteUserRequest) (dbplugin.DeleteUserResponse, error) {
	// Don't let anyone write the config while we're using it for our current client.
	minio.mux.RLock()
	defer minio.mux.RUnlock()

	client, err := BuildClient(ctx, minio.config)
	if err != nil {
		return dbplugin.DeleteUserResponse{}, fmt.Errorf("can't create madmin client: %w", err)
	}

	if err := client.DeleteUser(ctx, req.Username); err != nil {
		return dbplugin.DeleteUserResponse{}, fmt.Errorf("unable to delete user %s, %w", req.Username, err)
	}

	return dbplugin.DeleteUserResponse{}, nil
}

// Close for Minio is a NOOP, nothing to close
func (minio *Minio) Close() error {
	return nil
}

// SecretValues is used by some error-sanitizing middleware in Vault that basically
// replaces the keys in the map with the values given so they're not leaked via
// error messages.
func (minio *Minio) SecretValues() map[string]string {
	minio.mux.RLock()
	defer minio.mux.RUnlock()

	replacements := make(map[string]string)

	for _, secretKey := range []string{"password"} {
		vIfc, found := minio.config[secretKey]
		if !found {
			continue
		}

		secretVal, ok := vIfc.(string)
		if !ok {
			continue
		}

		// So, supposing a password of "0pen5e5ame",
		// this will cause that string to get replaced with "[password]".
		replacements[secretVal] = "[" + secretKey + "]"
	}

	return replacements
}

func newCreationStatement(statements dbplugin.Statements) (*CreationStatement, error) {
	if len(statements.Commands) == 0 {
		return nil, dbutil.ErrEmptyCreationStatement
	}

	if len(statements.Commands) > 1 {
		return nil, fmt.Errorf("only 1 creation statement supported for creation")
	}

	stmt := &CreationStatement{}

	if err := json.Unmarshal([]byte(statements.Commands[0]), stmt); err != nil {
		return nil, fmt.Errorf("unable to unmarshal %s: %w", []byte(statements.Commands[0]), err)
	}

	return stmt, nil
}
