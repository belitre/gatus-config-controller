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
				fields := []interface{}{"httproute", key, "selector", i}
				sel := selectors[i]
				if sel.Namespaces != nil {
					fields = append(fields, "namespace", route.Namespace, "namespaceInclude", sel.Namespaces.Include, "namespaceExclude", sel.Namespaces.Exclude)
				}
				if sel.Labels != nil {
					fields = append(fields, "routeLabels", route.Labels, "labelInclude", sel.Labels.Include, "labelExclude", sel.Labels.Exclude)
				}
				if sel.Annotations != nil {
					fields = append(fields, "routeAnnotations", route.Annotations, "annotationInclude", sel.Annotations.Include, "annotationExclude", sel.Annotations.Exclude)
				}
				if sel.ParentRefs != nil {
					fields = append(fields, "routeParentRefs", route.Spec.ParentRefs, "parentRefInclude", sel.ParentRefs.Include, "parentRefExclude", sel.ParentRefs.Exclude)
				}
				log.Info("httproute matched selector", fields...)
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
		log.V(1).Info("httproute did not match selector: namespace filter", "httproute", key, "selector", selectorIdx, "namespace", route.Namespace, "include", sel.Namespaces.Include, "exclude", sel.Namespaces.Exclude)
		return false
	}
	if sel.Labels != nil && !matchKeyValueFilter(route.Labels, sel.Labels) {
		log.V(1).Info("httproute did not match selector: label filter", "httproute", key, "selector", selectorIdx, "routeLabels", route.Labels, "include", sel.Labels.Include, "exclude", sel.Labels.Exclude)
		return false
	}
	if sel.Annotations != nil && !matchKeyValueFilter(route.Annotations, sel.Annotations) {
		log.V(1).Info("httproute did not match selector: annotation filter", "httproute", key, "selector", selectorIdx, "routeAnnotations", route.Annotations, "include", sel.Annotations.Include, "exclude", sel.Annotations.Exclude)
		return false
	}
	if sel.ParentRefs != nil && !matchParentRefFilter(route.Spec.ParentRefs, sel.ParentRefs) {
		log.V(1).Info("httproute did not match selector: parentRef filter", "httproute", key, "selector", selectorIdx, "routeParentRefs", route.Spec.ParentRefs, "include", sel.ParentRefs.Include, "exclude", sel.ParentRefs.Exclude)
		return false
	}
	return true
}

func matchParentRefFilter(refs []gwv1.ParentReference, f *config.ParentRefFilter) bool {
	if len(f.Include) > 0 {
		found := false
		for _, ref := range refs {
			for _, sel := range f.Include {
				if matchParentRef(ref, sel) {
					found = true
					break
				}
			}
			if found {
				break
			}
		}
		if !found {
			return false
		}
	}
	for _, ref := range refs {
		for _, sel := range f.Exclude {
			if matchParentRef(ref, sel) {
				return false
			}
		}
	}
	return true
}

func matchParentRef(ref gwv1.ParentReference, sel config.ParentRefSelector) bool {
	if sel.Name != "" && string(ref.Name) != sel.Name {
		return false
	}
	if sel.Namespace != "" {
		if ref.Namespace == nil || string(*ref.Namespace) != sel.Namespace {
			return false
		}
	}
	if sel.Group != "" {
		if ref.Group == nil || string(*ref.Group) != sel.Group {
			return false
		}
	}
	if sel.Kind != "" {
		if ref.Kind == nil || string(*ref.Kind) != sel.Kind {
			return false
		}
	}
	if sel.SectionName != "" {
		if ref.SectionName == nil || string(*ref.SectionName) != sel.SectionName {
			return false
		}
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
