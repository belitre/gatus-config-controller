package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	DefaultChecks []CheckTemplate   `yaml:"defaultChecks,omitempty"`
	Ingresses     []IngressSelector `yaml:"ingresses,omitempty"`
}

// CheckTemplate defines one Gatus check to generate per host.
// Scheme and NameSuffix together identify the check within an ingress.
type CheckTemplate struct {
	NameSuffix        string   `yaml:"nameSuffix,omitempty"`
	Scheme            string   `yaml:"scheme"` // "http" or "https"
	Interval          string   `yaml:"interval,omitempty"`
	Conditions        []string `yaml:"conditions,omitempty"`
	NoFollowRedirects bool     `yaml:"noFollowRedirects,omitempty"`
}

type IngressSelector struct {
	Namespaces     *StringFilter   `yaml:"namespaces,omitempty"`
	IngressClasses *StringFilter   `yaml:"ingressClasses,omitempty"`
	Labels         *KeyValueFilter `yaml:"labels,omitempty"`
	Annotations    *KeyValueFilter `yaml:"annotations,omitempty"`
	// Checks overrides DefaultChecks for ingresses matched by this selector.
	Checks []CheckTemplate `yaml:"checks,omitempty"`
}

type StringFilter struct {
	Include []string `yaml:"include,omitempty"`
	Exclude []string `yaml:"exclude,omitempty"`
}

type KeyValueFilter struct {
	Include []KeyValue `yaml:"include,omitempty"`
	Exclude []KeyValue `yaml:"exclude,omitempty"`
}

type KeyValue struct {
	Key   string `yaml:"key"`
	Value string `yaml:"value"`
}

func Load(path string) (*Config, error) {
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
