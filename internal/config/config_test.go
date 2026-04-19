package config_test

import (
	"os"
	"testing"

	"github.com/belitre/gatus-config-controller/internal/config"
)

func TestLoad(t *testing.T) {
	yaml := `
ingresses:
  - namespaces:
      include:
        - default
  - ingressClasses:
      include:
        - nginx
  - ingressClasses:
      include:
        - apps
    labels:
      exclude:
        - key: x
          value: "y"
`
	f, err := os.CreateTemp("", "config-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	_, _ = f.WriteString(yaml)
	f.Close()

	cfg, err := config.Load(f.Name())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Ingresses) != 3 {
		t.Fatalf("expected 3 selectors, got %d", len(cfg.Ingresses))
	}

	sel0 := cfg.Ingresses[0]
	if sel0.Namespaces == nil || len(sel0.Namespaces.Include) != 1 || sel0.Namespaces.Include[0] != "default" {
		t.Errorf("unexpected selector 0 namespaces: %+v", sel0.Namespaces)
	}

	sel1 := cfg.Ingresses[1]
	if sel1.IngressClasses == nil || len(sel1.IngressClasses.Include) != 1 || sel1.IngressClasses.Include[0] != "nginx" {
		t.Errorf("unexpected selector 1 ingressClasses: %+v", sel1.IngressClasses)
	}

	sel2 := cfg.Ingresses[2]
	if sel2.IngressClasses == nil || len(sel2.IngressClasses.Include) != 1 || sel2.IngressClasses.Include[0] != "apps" {
		t.Errorf("unexpected selector 2 ingressClasses: %+v", sel2.IngressClasses)
	}
	if sel2.Labels == nil || len(sel2.Labels.Exclude) != 1 {
		t.Fatalf("unexpected selector 2 labels: %+v", sel2.Labels)
	}
	if sel2.Labels.Exclude[0].Key != "x" || sel2.Labels.Exclude[0].Value != "y" {
		t.Errorf("unexpected label exclude: %+v", sel2.Labels.Exclude[0])
	}
}

func TestLoad_FileNotFound(t *testing.T) {
	_, err := config.Load("/nonexistent/path.yaml")
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestLoad_InvalidYAML(t *testing.T) {
	f, err := os.CreateTemp("", "config-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	_, _ = f.WriteString("{ invalid: [yaml")
	f.Close()

	_, err = config.Load(f.Name())
	if err == nil {
		t.Error("expected error for invalid YAML")
	}
}
