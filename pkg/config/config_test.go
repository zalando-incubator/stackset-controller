package config

import (
	"os"
	"reflect"
	"testing"
)

func TestReadConfig(t *testing.T) {
	for _, tc := range []struct {
		configFile string
		expected   *Config
	}{
		{
			configFile: `
synchronize-ingress-annotations:
  - "alb.ingress.kubernetes.io/ip-address-type"
  - "zalando.org/aws-load-balancer-ssl-cert"
  - "zalando.org/aws-load-balancer-scheme"
  - "kubernetes.io/ingress.class"
`,
			expected: &Config{
				SynchronizeIngressAnnotations: []string{
					"alb.ingress.kubernetes.io/ip-address-type",
					"zalando.org/aws-load-balancer-ssl-cert",
					"zalando.org/aws-load-balancer-scheme",
					"kubernetes.io/ingress.class",
				},
			},
		},
		{
			configFile: `synchronize-ingress-annotations: []`,
			expected: &Config{
				SynchronizeIngressAnnotations: []string{},
			},
		},
		{
			configFile: `another-field: []`,
			expected:   &Config{},
		},
	} {
		configFile, err := generateTempConfig(tc.configFile)
		if err != nil {
			t.Errorf("Error generating temp config file: %v", err)
			continue
		}
		defer os.Remove(configFile)

		config, err := ReadConfig(configFile)
		if err != nil {
			t.Errorf("Error reding configuration: %v", err)
			continue
		}

		if !reflect.DeepEqual(config, tc.expected) {
			t.Errorf("Expected config to be %v, got %v", tc.expected, config)
		}
	}
}

func generateTempConfig(contents string) (string, error) {
	f, err := os.CreateTemp("", "testconfig.yaml")
	if err != nil {
		return "", err
	}

	_, err = f.Write([]byte(contents))
	if err != nil {
		return "", err
	}

	return f.Name(), nil
}
