package main

import (
	"log"
	"os"

	dbplugin "github.com/hashicorp/vault/sdk/database/dbplugin/v5"
	minio "github.com/ohayocorp/vault-minio-database-plugin"
)

func main() {
	if err := Run(); err != nil {
		log.Println(err)
		os.Exit(1)
	}
}

// Run starts serving the plugin
func Run() error {
	dbplugin.ServeMultiplex(minio.New)
	return nil
}
