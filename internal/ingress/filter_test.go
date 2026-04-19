package ingress_test

import (
	"testing"

	"github.com/go-logr/logr"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/belitre/gatus-config-controller/internal/config"
	"github.com/belitre/gatus-config-controller/internal/ingress"
)

func makeIngress(name, namespace, class string, labels, annotations map[string]string) networkingv1.Ingress {
	ing := networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   namespace,
			Labels:      labels,
			Annotations: annotations,
		},
	}
	if class != "" {
		ing.Spec.IngressClassName = &class
	}
	return ing
}

func matchKeys(results []ingress.MatchResult) []string {
	out := make([]string, len(results))
	for i, r := range results {
		out[i] = r.Ingress.Namespace + "/" + r.Ingress.Name
	}
	return out
}

// containsSame checks that got and want have exactly the same keys with the same frequency.
func containsSame(got []string, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	freq := make(map[string]int, len(want))
	for _, k := range want {
		freq[k]++
	}
	for _, k := range got {
		freq[k]--
		if freq[k] < 0 {
			return false
		}
	}
	return true
}

var testIngresses = []networkingv1.Ingress{
	makeIngress("a", "default", "nginx", map[string]string{"app": "web"}, nil),
	makeIngress("b", "default", "nginx", map[string]string{"app": "api"}, nil),
	makeIngress("c", "kube-system", "nginx", nil, nil),
	makeIngress("d", "default", "apps", map[string]string{"env": "prod"}, nil),
	makeIngress("e", "staging", "apps", map[string]string{"env": "staging"}, nil),
	makeIngress("f", "default", "", nil, map[string]string{"visibility": "internal"}),
}

func TestFilter(t *testing.T) {
	tests := []struct {
		name      string
		selectors []config.IngressSelector
		// want lists the expected ingress keys (namespace/name); duplicates mean the
		// same ingress was matched by multiple selectors.
		want []string
	}{
		{
			name:      "nil selectors returns all",
			selectors: nil,
			want:      []string{"default/a", "default/b", "kube-system/c", "default/d", "staging/e", "default/f"},
		},
		{
			name:      "empty selectors returns none",
			selectors: []config.IngressSelector{},
			want:      []string{},
		},
		{
			name:      "empty selector matches all",
			selectors: []config.IngressSelector{{}},
			want:      []string{"default/a", "default/b", "kube-system/c", "default/d", "staging/e", "default/f"},
		},
		{
			name:      "namespace include",
			selectors: []config.IngressSelector{{Namespaces: &config.StringFilter{Include: []string{"default"}}}},
			want:      []string{"default/a", "default/b", "default/d", "default/f"},
		},
		{
			name:      "namespace exclude",
			selectors: []config.IngressSelector{{Namespaces: &config.StringFilter{Exclude: []string{"default"}}}},
			want:      []string{"kube-system/c", "staging/e"},
		},
		{
			name:      "namespace include and exclude",
			selectors: []config.IngressSelector{{Namespaces: &config.StringFilter{Include: []string{"default", "staging"}, Exclude: []string{"staging"}}}},
			want:      []string{"default/a", "default/b", "default/d", "default/f"},
		},
		{
			name:      "ingressClass include",
			selectors: []config.IngressSelector{{IngressClasses: &config.StringFilter{Include: []string{"nginx"}}}},
			want:      []string{"default/a", "default/b", "kube-system/c"},
		},
		{
			name:      "ingressClass exclude",
			selectors: []config.IngressSelector{{IngressClasses: &config.StringFilter{Exclude: []string{"nginx"}}}},
			want:      []string{"default/d", "staging/e", "default/f"},
		},
		{
			name:      "ingressClass empty string (no class set)",
			selectors: []config.IngressSelector{{IngressClasses: &config.StringFilter{Include: []string{""}}}},
			want:      []string{"default/f"},
		},
		{
			name:      "labels include",
			selectors: []config.IngressSelector{{Labels: &config.KeyValueFilter{Include: []config.KeyValue{{Key: "app", Value: "web"}}}}},
			want:      []string{"default/a"},
		},
		{
			name:      "labels exclude",
			selectors: []config.IngressSelector{{Labels: &config.KeyValueFilter{Exclude: []config.KeyValue{{Key: "env", Value: "staging"}}}}},
			want:      []string{"default/a", "default/b", "kube-system/c", "default/d", "default/f"},
		},
		{
			name:      "annotations include",
			selectors: []config.IngressSelector{{Annotations: &config.KeyValueFilter{Include: []config.KeyValue{{Key: "visibility", Value: "internal"}}}}},
			want:      []string{"default/f"},
		},
		{
			name:      "annotations exclude",
			selectors: []config.IngressSelector{{Annotations: &config.KeyValueFilter{Exclude: []config.KeyValue{{Key: "visibility", Value: "internal"}}}}},
			want:      []string{"default/a", "default/b", "kube-system/c", "default/d", "staging/e"},
		},
		{
			name: "ingressClass include + labels exclude (user example)",
			selectors: []config.IngressSelector{{
				IngressClasses: &config.StringFilter{Include: []string{"apps"}},
				Labels:         &config.KeyValueFilter{Exclude: []config.KeyValue{{Key: "env", Value: "staging"}}},
			}},
			want: []string{"default/d"},
		},
		{
			// Each ingress matching multiple selectors appears once per selector.
			name: "ingress matching multiple selectors appears per match",
			selectors: []config.IngressSelector{
				{Namespaces: &config.StringFilter{Include: []string{"default"}}},
				{IngressClasses: &config.StringFilter{Include: []string{"apps"}}},
			},
			// default/a matches selector 0 only (class=nginx, not apps)
			// default/b matches selector 0 only
			// kube-system/c matches neither
			// default/d matches selector 0 AND selector 1 → appears twice
			// staging/e matches selector 1 only
			// default/f matches selector 0 only
			want: []string{"default/a", "default/b", "default/d", "default/d", "staging/e", "default/f"},
		},
		{
			// Same ingress matched by two selectors with the same config → two results
			// (endpoint dedup by content happens in the controller, not here).
			name: "same ingress matched by two selectors produces two results",
			selectors: []config.IngressSelector{
				{Namespaces: &config.StringFilter{Include: []string{"default"}}},
				{IngressClasses: &config.StringFilter{Include: []string{"nginx"}}},
			},
			// default/a (ns=default, class=nginx): matches both → 2
			// default/b (ns=default, class=nginx): matches both → 2
			// kube-system/c (class=nginx): matches selector 1 only → 1
			// default/d (ns=default, class=apps): matches selector 0 only → 1
			// staging/e: matches neither → 0
			// default/f (ns=default): matches selector 0 only → 1
			want: []string{"default/a", "default/a", "default/b", "default/b", "kube-system/c", "default/d", "default/f"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ingress.Filter(logr.Discard(), testIngresses, tt.selectors)
			if !containsSame(matchKeys(got), tt.want) {
				t.Errorf("got %v, want %v", matchKeys(got), tt.want)
			}
		})
	}
}

func TestFilterSelectorAssociation(t *testing.T) {
	sel0 := config.IngressSelector{Namespaces: &config.StringFilter{Include: []string{"default"}}}
	sel1 := config.IngressSelector{IngressClasses: &config.StringFilter{Include: []string{"nginx"}}}

	results := ingress.Filter(logr.Discard(), testIngresses, []config.IngressSelector{sel0, sel1})

	for _, r := range results {
		key := r.Ingress.Namespace + "/" + r.Ingress.Name
		if r.Selector == nil {
			t.Errorf("ingress %s: expected non-nil Selector", key)
		}
	}

	// default/a should appear twice, once for each selector
	var aResults []ingress.MatchResult
	for _, r := range results {
		if r.Ingress.Name == "a" && r.Ingress.Namespace == "default" {
			aResults = append(aResults, r)
		}
	}
	if len(aResults) != 2 {
		t.Fatalf("default/a: expected 2 matches, got %d", len(aResults))
	}
	if aResults[0].Selector == aResults[1].Selector {
		t.Error("default/a: both matches point to the same selector, expected different ones")
	}
}
