package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	DefaultChecks []CheckTemplate     `yaml:"defaultChecks,omitempty"`
	Ingresses     []IngressSelector   `yaml:"ingresses,omitempty"`
	HTTPRoutes    []HTTPRouteSelector `yaml:"httpRoutes,omitempty"`
	StaticConfig  any         `yaml:"staticConfig,omitempty"`
}

// CheckTemplate defines one Gatus check to generate per host.
// Set either Scheme (HTTP/HTTPS check) or DNS (DNS check), not both.
// Any additional Gatus endpoint fields (e.g. group, alerts, headers, ui) can be
// set directly alongside the known fields — they are passed through as-is.
type CheckTemplate struct {
	NameSuffix        string         `yaml:"nameSuffix,omitempty"`
	Scheme            string         `yaml:"scheme,omitempty"`
	Interval          string         `yaml:"interval,omitempty"`
	Conditions        []string       `yaml:"conditions,omitempty"`
	NoFollowRedirects bool           `yaml:"noFollowRedirects,omitempty"`
	DNS               *DNSCheck      `yaml:"dns,omitempty"`
	Extra             map[string]any `yaml:",inline"`
}

// DNSCheck configures a Gatus DNS check. The discovered hostname is used as the query-name.
type DNSCheck struct {
	NameServer string `yaml:"nameServer"`
	QueryType  string `yaml:"queryType"`
}

// HTTPRouteSelector selects HTTPRoute resources.
type HTTPRouteSelector struct {
	Namespaces  *StringFilter    `yaml:"namespaces,omitempty"`
	Labels      *KeyValueFilter  `yaml:"labels,omitempty"`
	Annotations *KeyValueFilter  `yaml:"annotations,omitempty"`
	ParentRefs  *ParentRefFilter `yaml:"parentRefs,omitempty"`
	Checks      []CheckTemplate  `yaml:"checks,omitempty"`
}

// ParentRefFilter filters HTTPRoutes by their parentRefs (e.g. which Gateway they attach to).
type ParentRefFilter struct {
	Include []ParentRefSelector `yaml:"include,omitempty"`
	Exclude []ParentRefSelector `yaml:"exclude,omitempty"`
}

// ParentRefSelector matches a parentRef entry. All non-empty fields must match.
// Empty fields act as wildcards.
type ParentRefSelector struct {
	Group       string `yaml:"group,omitempty"`
	Kind        string `yaml:"kind,omitempty"`
	Name        string `yaml:"name,omitempty"`
	Namespace   string `yaml:"namespace,omitempty"`
	SectionName string `yaml:"sectionName,omitempty"`
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
