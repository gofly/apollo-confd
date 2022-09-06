package main

import (
	"os"

	"gopkg.in/yaml.v2"
)

type ApolloConfig struct {
	API        string   `yaml:"api"`
	AppID      string   `yaml:"app_id"`
	Cluster    string   `yaml:"cluster"`
	Namespaces []string `yaml:"namespaces"`
	Secret     string   `yaml:"secret"`
}

type WatchConfig struct {
	OnChange string `yaml:"onchange"`
	Groups   []struct {
		Path string   `yaml:"path"`
		Keys []string `yaml:"keys"`
	} `yaml:"groups"`
}

type ApolloConfdConfig struct {
	Apollo ApolloConfig  `yaml:"apollo"`
	Watch  []WatchConfig `yaml:"watch"`
}

func LoadApolloConfdConfig(filePath string) (*ApolloConfdConfig, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	config := &ApolloConfdConfig{}
	err = yaml.NewDecoder(f).Decode(config)
	return config, err
}
