package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	SynchronizeIngressAnnotations []string `yaml:"synchronize-ingress-annotations,omitempty"`
}

func ReadConfig(filename string) (*Config, error) {
	buf, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	res := &Config{}
	err = yaml.Unmarshal(buf, res)
	if err != nil {
		return nil, err
	}

	return res, nil
}
