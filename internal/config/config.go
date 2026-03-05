package config

import (
	"fmt"
	"os"
)


type Config struct {
	HTTPPort string
	DBDriver string
	DBDSN    string
}

func Load() (*Config, error) {
	httpPort := getEnv("TASKFLOW_HTTP_PORT", "8080")
	driver := getEnv("TASKFLOW_DB_DRIVER", "postgres")
	dsn := os.Getenv("TASKFLOW_DB_DSN")

	if driver == "postgres" && dsn == "" {
		// provide a sensible default template for postgres; user should override
		dsn = "host=localhost user=postgres password=27114 dbname=taskflow port=5432 sslmode=disable TimeZone=UTC"
	}

	if httpPort == "" {
		return nil, fmt.Errorf("TASKFLOW_HTTP_PORT is required")
	}

	return &Config{
		HTTPPort: httpPort,
		DBDriver: driver,
		DBDSN:    dsn,
	}, nil
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

