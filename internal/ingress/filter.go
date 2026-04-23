package ingress

import (
	"github.com/go-logr/logr"
	networkingv1 "k8s.io/api/networking/v1"

	"github.com/belitre/gatus-config-controller/internal/config"
)

// MatchResult pairs an ingress with the selector that matched it.
// Selector is nil when no selectors are configured (select-all mode).
type MatchResult struct {
	Ingress  networkingv1.Ingress
	Selector *config.IngressSelector
}

// Filter returns one MatchResult per (ingress, selector) pair that matches.
// The same ingress can appear multiple times if matched by different selectors.
// If selectors is nil, all ingresses are returned each with a nil Selector.
func Filter(log logr.Logger, ingresses []networkingv1.Ingress, selectors []config.IngressSelector) []MatchResult {
	if selectors == nil {
		log.V(1).Info("no selectors configured, all ingresses selected", "count", len(ingresses))
		results := make([]MatchResult, len(ingresses))
		for i, ing := range ingresses {
			results[i] = MatchResult{Ingress: ing}
		}
		return results
	}

	var results []MatchResult
	for _, ing := range ingresses {
		key := ing.Namespace + "/" + ing.Name
		log.V(1).Info("checking ingress", "ingress", key)
		matched := false
		for i := range selectors {
			if matchesSelector(log, i, ing, selectors[i]) {
				fields := []interface{}{"ingress", key, "selector", i}
				sel := selectors[i]
				if sel.Namespaces != nil {
					fields = append(fields, "namespace", ing.Namespace, "namespaceInclude", sel.Namespaces.Include, "namespaceExclude", sel.Namespaces.Exclude)
				}
				if sel.IngressClasses != nil {
					class := ""
					if ing.Spec.IngressClassName != nil {
						class = *ing.Spec.IngressClassName
					}
					fields = append(fields, "ingressClass", class, "ingressClassInclude", sel.IngressClasses.Include, "ingressClassExclude", sel.IngressClasses.Exclude)
				}
				if sel.Labels != nil {
					fields = append(fields, "ingressLabels", ing.Labels, "labelInclude", sel.Labels.Include, "labelExclude", sel.Labels.Exclude)
				}
				if sel.Annotations != nil {
					fields = append(fields, "ingressAnnotations", ing.Annotations, "annotationInclude", sel.Annotations.Include, "annotationExclude", sel.Annotations.Exclude)
				}
				log.Info("ingress matched selector", fields...)
				results = append(results, MatchResult{Ingress: ing, Selector: &selectors[i]})
				matched = true
			}
		}
		if !matched {
			log.V(1).Info("ingress discarded, no selector matched", "ingress", key)
		}
	}
	return results
}

func matchesSelector(log logr.Logger, selectorIdx int, ing networkingv1.Ingress, sel config.IngressSelector) bool {
	key := ing.Namespace + "/" + ing.Name
	if sel.Namespaces != nil && !matchStringFilter(ing.Namespace, sel.Namespaces) {
		log.V(1).Info("ingress did not match selector: namespace filter", "ingress", key, "selector", selectorIdx, "namespace", ing.Namespace, "include", sel.Namespaces.Include, "exclude", sel.Namespaces.Exclude)
		return false
	}
	if sel.IngressClasses != nil {
		class := ""
		if ing.Spec.IngressClassName != nil {
			class = *ing.Spec.IngressClassName
		}
		if !matchStringFilter(class, sel.IngressClasses) {
			log.V(1).Info("ingress did not match selector: ingressClass filter", "ingress", key, "selector", selectorIdx, "ingressClass", class, "include", sel.IngressClasses.Include, "exclude", sel.IngressClasses.Exclude)
			return false
		}
	}
	if sel.Labels != nil && !matchKeyValueFilter(ing.Labels, sel.Labels) {
		log.V(1).Info("ingress did not match selector: label filter", "ingress", key, "selector", selectorIdx, "ingressLabels", ing.Labels, "include", sel.Labels.Include, "exclude", sel.Labels.Exclude)
		return false
	}
	if sel.Annotations != nil && !matchKeyValueFilter(ing.Annotations, sel.Annotations) {
		log.V(1).Info("ingress did not match selector: annotation filter", "ingress", key, "selector", selectorIdx, "ingressAnnotations", ing.Annotations, "include", sel.Annotations.Include, "exclude", sel.Annotations.Exclude)
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
