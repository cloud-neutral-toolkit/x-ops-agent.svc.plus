package main

import (
	"flag"
	"log"
	"net/http"
	"os"

	"gopkg.in/yaml.v3"

	"github.com/yourname/ops-agent-poc/api"
)

type Config struct {
	Inputs struct {
		DB struct {
			PGURL        string   `yaml:"pgurl"`
			OTelEndpoint []string `yaml:"otel_endpoint"`
		} `yaml:"db"`
	} `yaml:"inputs"`
	Outputs struct {
		API struct {
			Listen string `yaml:"listen"`
			Type   string `yaml:"type"`
		} `yaml:"api"`
		GitOps []struct {
			RepoURL string `yaml:"repoUrl"`
			Token   string `yaml:"token"`
		} `yaml:"gitops"`
	} `yaml:"outputs"`
	Models struct {
		Embedder struct {
			Models   string `yaml:"models"`
			Endpoint string `yaml:"endpoint"`
		} `yaml:"embedder"`
		Generator struct {
			Models   []string `yaml:"models"`
			Endpoint string   `yaml:"endpoint"`
		} `yaml:"generator"`
	} `yaml:"models"`
}

func loadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func main() {
	cfgPath := flag.String("config", "/etc/XOpsAgent.yaml", "path to config file")
	flag.Parse()

	cfg, err := loadConfig(*cfgPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	mux := http.NewServeMux()
	api.RegisterRoutes(mux)

	listen := cfg.Outputs.API.Listen
	if listen == "" {
		listen = ":8080"
	}

	log.Printf("XOpsAgent daemon listening on %s", listen)
	if err := http.ListenAndServe(listen, mux); err != nil {
		log.Fatalf("listen: %v", err)
	}
}
