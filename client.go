package vaultminio

import (
	"context"
	"fmt"
	"strconv"
	"time"

	madmin "github.com/minio/madmin-go/v2"
)

// This lightweight client implements only the methods needed for this secrets engine.

type MinioClientConfig struct {
	Username, Password, BaseURL string
	UseSSL                      bool
}

type MinioClient struct {
	Client madmin.AdminClient
	Config MinioClientConfig
}

// BuildClient is a helper method for building a client from the present config,
// which is done often.
func BuildClient(ctx context.Context, config map[string]interface{}) (*MinioClient, error) {
	// We can presume these required fields are provided by strings
	// because they're validated in Init.
	useSSL, _ := strconv.ParseBool(config["useSSL"].(string))

	clientConfig := &MinioClientConfig{
		Username: config["username"].(string),
		Password: config["password"].(string),
		BaseURL:  config["url"].(string),
		UseSSL:   useSSL,
	}

	client, err := madmin.New(clientConfig.BaseURL, clientConfig.Username, clientConfig.Password, clientConfig.UseSSL)

	if err != nil {
		return nil, err
	}

	minioClient := &MinioClient{
		Client: *client,
		Config: *clientConfig,
	}

	return minioClient, nil
}

func (c *MinioClient) CreateUser(ctx context.Context, username string, password string, stmt *CreationStatement, expiration time.Time) error {
	err := c.Client.AddUser(ctx, username, password)

	if err != nil {
		return fmt.Errorf("can't create user: %s, %w", username, err)
	}

	err = c.Client.SetPolicy(ctx, stmt.Policy, username, false)

	if err != nil {
		return fmt.Errorf("can't set policy %s for user: %s, %w", stmt.Policy, username, err)
	}

	return nil
}

func (c *MinioClient) UpdateUser(ctx context.Context, username string, password string) error {
	err := c.Client.SetUser(ctx, username, password, madmin.AccountEnabled)

	if err != nil {
		return fmt.Errorf("can't change password for user: %s, %w", username, err)
	}

	return nil
}

func (c *MinioClient) DeleteUser(ctx context.Context, username string) error {
	err := c.Client.RemoveUser(ctx, username)

	if err != nil {
		return fmt.Errorf("can't delete user: %s, %w", username, err)
	}

	return nil
}
