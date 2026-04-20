package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	DefaultChecks []CheckTemplate     `yaml:"defaultChecks,omitempty"`
	Ingresses     []IngressSelector   `yaml:"ingresses,omitempty"`
	HTTPRoutes    []HTTPRouteSelector `yaml:"httpRoutes,omitempty"`
	StaticConfig  interface{}         `yaml:"staticConfig,omitempty"`
}

// CheckTemplate defines one Gatus check to generate per host.
// Set either Scheme (HTTP/HTTPS check) or DNS (DNS check), not both.
type CheckTemplate struct {
	NameSuffix        string    `yaml:"nameSuffix,omitempty"`
	Scheme            string    `yaml:"scheme,omitempty"`
	Interval          string    `yaml:"interval,omitempty"`
	Conditions        []string  `yaml:"conditions,omitempty"`
	NoFollowRedirects bool      `yaml:"noFollowRedirects,omitempty"`
	DNS               *DNSCheck `yaml:"dns,omitempty"`
}

// DNSCheck configures a Gatus DNS check. The discovered hostname is used as the query-name.
type DNSCheck struct {
	NameServer string `yaml:"nameServer"`
	QueryType  string `yaml:"queryType"`
}

// HTTPRouteSelector selects HTTPRoute resources. Unlike IngressSelector there is no
// class filter — any HTTPRoute with at least one hostname qualifies.
type HTTPRouteSelector struct {
	Namespaces  *StringFilter   `yaml:"namespaces,omitempty"`
	Labels      *KeyValueFilter `yaml:"labels,omitempty"`
	Annotations *KeyValueFilter `yaml:"annotations,omitempty"`
	Checks      []CheckTemplate `yaml:"checks,omitempty"`
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
