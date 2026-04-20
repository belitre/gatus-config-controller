package httproute

import (
	"github.com/go-logr/logr"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/belitre/gatus-config-controller/internal/config"
)

// MatchResult pairs an HTTPRoute with the selector that matched it.
// Selector is nil when no selectors are configured (select-all mode).
type MatchResult struct {
	Route    gwv1.HTTPRoute
	Selector *config.HTTPRouteSelector
}

// Filter returns one MatchResult per (route, selector) pair that matches.
// The same route can appear multiple times if matched by different selectors.
// If selectors is nil, all routes with at least one hostname are returned with a nil Selector.
func Filter(log logr.Logger, routes []gwv1.HTTPRoute, selectors []config.HTTPRouteSelector) []MatchResult {
	if selectors == nil {
		log.V(1).Info("no httproute selectors configured, all httproutes selected", "count", len(routes))
		results := make([]MatchResult, len(routes))
		for i, r := range routes {
			results[i] = MatchResult{Route: r}
		}
		return results
	}

	var results []MatchResult
	for _, route := range routes {
		key := route.Namespace + "/" + route.Name
		log.V(1).Info("checking httproute", "httproute", key)
		matched := false
		for i := range selectors {
			if matchesSelector(log, i, route, selectors[i]) {
				log.V(1).Info("httproute matched selector", "httproute", key, "selector", i)
				results = append(results, MatchResult{Route: route, Selector: &selectors[i]})
				matched = true
			}
		}
		if !matched {
			log.V(1).Info("httproute discarded, no selector matched", "httproute", key)
		}
	}
	return results
}

func matchesSelector(log logr.Logger, selectorIdx int, route gwv1.HTTPRoute, sel config.HTTPRouteSelector) bool {
	key := route.Namespace + "/" + route.Name
	if sel.Namespaces != nil && !matchStringFilter(route.Namespace, sel.Namespaces) {
		log.V(1).Info("httproute did not match selector: namespace filter", "httproute", key, "selector", selectorIdx, "namespace", route.Namespace)
		return false
	}
	if sel.Labels != nil && !matchKeyValueFilter(route.Labels, sel.Labels) {
		log.V(1).Info("httproute did not match selector: label filter", "httproute", key, "selector", selectorIdx)
		return false
	}
	if sel.Annotations != nil && !matchKeyValueFilter(route.Annotations, sel.Annotations) {
		log.V(1).Info("httproute did not match selector: annotation filter", "httproute", key, "selector", selectorIdx)
		return false
	}
	return true
}

func matchStringFilter(value string, f *config.StringFilter) bool {
	if len(f.Include) > 0 {
		found := false
		for _, v := range f.Include {
			if v == value {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	for _, v := range f.Exclude {
		if v == value {
			return false
		}
	}
	return true
}

func matchKeyValueFilter(kv map[string]string, f *config.KeyValueFilter) bool {
	for _, req := range f.Include {
		if v, ok := kv[req.Key]; !ok || v != req.Value {
			return false
		}
	}
	for _, req := range f.Exclude {
		if v, ok := kv[req.Key]; ok && v == req.Value {
			return false
		}
	}
	return true
}
