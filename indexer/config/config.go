package config

import (
	"os"

	"github.com/BurntSushi/toml"
)

// The go struct representation of the `indexer.toml` file used to configure the indexer
type Config struct {
	Chain   Chain
	RPCs    RPCs `toml:"rpcs"`
	DB      DB
	API     API
	Metrics Metrics
}

// Configuration of the chain being indexed
type Chain struct {
	// Configure known chains with the l2 chain id
	Preset int
}

// Configuration of RPC urls to connect to
type RPCs struct {
	L1RPC string `toml:"l1-rpc"`
	L2RPC string `toml:"l2-rpc"`
}

// Configuration of the postgres database to connect to
type DB struct {
	Host     string
	Port     int
	User     string
	Password string
}

// Configuration of the API server
type API struct {
	Host string
	Port int
}

// Configuration of the Metrics server
type Metrics struct {
	Host string
	Port int
}

// Loads the `indexer.toml` config file from a given path
func LoadConfig(path string) (Config, error) {
	var conf Config

	// Read the config file.
	data, err := os.ReadFile(path)
	if err != nil {
		return conf, err
	}

	// Replace environment variables.
	data = []byte(os.ExpandEnv(string(data)))

	// Decode the TOML data.
	if _, err := toml.Decode(string(data), &conf); err != nil {
		return conf, err
	}

	return conf, nil
}
