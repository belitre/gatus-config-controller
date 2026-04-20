package httproute_test

import (
	"testing"

	"github.com/go-logr/logr"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/belitre/gatus-config-controller/internal/config"
	"github.com/belitre/gatus-config-controller/internal/httproute"
)

func makeRoute(name, namespace string, hostnames []string, labels, annotations map[string]string) gwv1.HTTPRoute {
	route := gwv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   namespace,
			Labels:      labels,
			Annotations: annotations,
		},
	}
	for _, h := range hostnames {
		route.Spec.Hostnames = append(route.Spec.Hostnames, gwv1.Hostname(h))
	}
	return route
}

func matchKeys(results []httproute.MatchResult) []string {
	out := make([]string, len(results))
	for i, r := range results {
		out[i] = r.Route.Namespace + "/" + r.Route.Name
	}
	return out
}

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

var testRoutes = []gwv1.HTTPRoute{
	makeRoute("a", "default", []string{"a.example.com"}, map[string]string{"app": "web"}, nil),
	makeRoute("b", "default", []string{"b.example.com"}, map[string]string{"app": "api"}, nil),
	makeRoute("c", "kube-system", []string{"c.example.com"}, nil, nil),
	makeRoute("d", "default", []string{"d.example.com"}, map[string]string{"env": "prod"}, nil),
	makeRoute("e", "staging", []string{"e.example.com"}, map[string]string{"env": "staging"}, nil),
	makeRoute("f", "default", []string{"f.example.com"}, nil, map[string]string{"visibility": "internal"}),
}

func TestFilter(t *testing.T) {
	tests := []struct {
		name      string
		selectors []config.HTTPRouteSelector
		want      []string
	}{
		{
			name:      "nil selectors returns all",
			selectors: nil,
			want:      []string{"default/a", "default/b", "kube-system/c", "default/d", "staging/e", "default/f"},
		},
		{
			name:      "empty selectors returns none",
			selectors: []config.HTTPRouteSelector{},
			want:      []string{},
		},
		{
			name:      "empty selector matches all",
			selectors: []config.HTTPRouteSelector{{}},
			want:      []string{"default/a", "default/b", "kube-system/c", "default/d", "staging/e", "default/f"},
		},
		{
			name:      "namespace include",
			selectors: []config.HTTPRouteSelector{{Namespaces: &config.StringFilter{Include: []string{"default"}}}},
			want:      []string{"default/a", "default/b", "default/d", "default/f"},
		},
		{
			name:      "namespace exclude",
			selectors: []config.HTTPRouteSelector{{Namespaces: &config.StringFilter{Exclude: []string{"default"}}}},
			want:      []string{"kube-system/c", "staging/e"},
		},
		{
			name:      "labels include",
			selectors: []config.HTTPRouteSelector{{Labels: &config.KeyValueFilter{Include: []config.KeyValue{{Key: "app", Value: "web"}}}}},
			want:      []string{"default/a"},
		},
		{
			name:      "labels exclude",
			selectors: []config.HTTPRouteSelector{{Labels: &config.KeyValueFilter{Exclude: []config.KeyValue{{Key: "env", Value: "staging"}}}}},
			want:      []string{"default/a", "default/b", "kube-system/c", "default/d", "default/f"},
		},
		{
			name:      "annotations exclude",
			selectors: []config.HTTPRouteSelector{{Annotations: &config.KeyValueFilter{Exclude: []config.KeyValue{{Key: "visibility", Value: "internal"}}}}},
			want:      []string{"default/a", "default/b", "kube-system/c", "default/d", "staging/e"},
		},
		{
			// Same route matched by two selectors appears once per selector.
			name: "route matching multiple selectors appears per match",
			selectors: []config.HTTPRouteSelector{
				{Namespaces: &config.StringFilter{Include: []string{"default"}}},
				{Labels: &config.KeyValueFilter{Include: []config.KeyValue{{Key: "env", Value: "prod"}}}},
			},
			// default/d matches both selectors → appears twice
			want: []string{"default/a", "default/b", "default/d", "default/d", "default/f"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := httproute.Filter(logr.Discard(), testRoutes, tt.selectors)
			if !containsSame(matchKeys(got), tt.want) {
				t.Errorf("got %v, want %v", matchKeys(got), tt.want)
			}
		})
	}
}
