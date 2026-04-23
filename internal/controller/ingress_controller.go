package controller

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/go-logr/logr"
	"gopkg.in/yaml.v3"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/belitre/gatus-config-controller/internal/config"
	ingressfilter "github.com/belitre/gatus-config-controller/internal/ingress"
	httproutefilter "github.com/belitre/gatus-config-controller/internal/httproute"
)

const (
	defaultInterval  = "60s"
	defaultCondition = "[STATUS] == 200"
)

// builtinChecks are used when no defaultChecks are configured.
var builtinChecks = []config.CheckTemplate{
	{
		Scheme:     "https",
		Interval:   defaultInterval,
		Conditions: []string{defaultCondition},
	},
}

type IngressReconciler struct {
	client.Client
	ConfigMapNamespace string
	ConfigMapName      string
	Selectors          []config.IngressSelector
	DefaultChecks      []config.CheckTemplate
	WatchHTTPRoutes    bool
	HTTPRouteSelectors []config.HTTPRouteSelector
	StaticConfig       any
}

type gatusClient struct {
	IgnoreRedirect bool `yaml:"ignore-redirect,omitempty"`
}

type gatusDNS struct {
	QueryName string `yaml:"query-name"`
	QueryType string `yaml:"query-type"`
}

type gatusEndpoint struct {
	Name       string                 `yaml:"name"`
	URL        string                 `yaml:"url"`
	Interval   string                 `yaml:"interval"`
	Conditions []string               `yaml:"conditions"`
	Client     *gatusClient           `yaml:"client,omitempty"`
	DNS        *gatusDNS              `yaml:"dns,omitempty"`
	Extra      map[string]any `yaml:",inline"`
}

type gatusConfig struct {
	Endpoints []gatusEndpoint `yaml:"endpoints"`
}

func (r *IngressReconciler) resolveChecks(selectorChecks []config.CheckTemplate) []config.CheckTemplate {
	if len(selectorChecks) > 0 {
		return selectorChecks
	}
	if len(r.DefaultChecks) > 0 {
		return r.DefaultChecks
	}
	return builtinChecks
}

func endpointKey(ep gatusEndpoint) string {
	target := ep.URL
	if ep.DNS != nil {
		target += "|" + ep.DNS.QueryName
	}
	return fmt.Sprintf("%s|%s|%s|%v", ep.Name, target, ep.Interval, ep.Conditions)
}

func buildEndpoint(name, host, path string, tmpl config.CheckTemplate) gatusEndpoint {
	interval := tmpl.Interval
	if interval == "" {
		interval = defaultInterval
	}
	conditions := tmpl.Conditions
	if len(conditions) == 0 {
		conditions = []string{defaultCondition}
	}
	ep := gatusEndpoint{
		Name:       name,
		Interval:   interval,
		Conditions: conditions,
		Extra:      tmpl.Extra,
	}
	if tmpl.DNS != nil {
		ep.URL = tmpl.DNS.NameServer
		ep.DNS = &gatusDNS{QueryName: host, QueryType: tmpl.DNS.QueryType}
	} else {
		u := url.URL{Scheme: tmpl.Scheme, Host: strings.TrimRight(host, "/"), Path: path}
		ep.URL = u.String()
		if tmpl.NoFollowRedirects {
			ep.Client = &gatusClient{IgnoreRedirect: true}
		}
	}
	return ep
}

// extractIngressPaths returns the paths defined in an ingress rule.
// If no paths are found it warns and defaults to "/".
// DNS check templates should ignore paths and use "/" directly.
func extractIngressPaths(log logr.Logger, rule networkingv1.IngressRule, key string) []string {
	if rule.HTTP == nil || len(rule.HTTP.Paths) == 0 {
		log.Info("ingress rule has no paths, defaulting to /", "ingress", key, "host", rule.Host)
		return []string{"/"}
	}
	paths := make([]string, 0, len(rule.HTTP.Paths))
	for _, p := range rule.HTTP.Paths {
		path := p.Path
		if path == "" {
			log.Info("ingress rule has empty path, defaulting to /", "ingress", key, "host", rule.Host)
			path = "/"
		}
		paths = append(paths, path)
	}
	return paths
}

// extractHTTPRoutePaths returns the unique concrete paths across all rule matches.
// RegularExpression paths are skipped with a warning and collapsed to "/".
// If no paths are found it warns and defaults to "/".
// DNS check templates should ignore paths and use "/" directly.
func extractHTTPRoutePaths(log logr.Logger, route gwv1.HTTPRoute, key string) []string {
	seen := make(map[string]struct{})
	var paths []string
	add := func(p string) {
		if _, ok := seen[p]; !ok {
			seen[p] = struct{}{}
			paths = append(paths, p)
		}
	}
	for _, rule := range route.Spec.Rules {
		if len(rule.Matches) == 0 {
			log.Info("httproute rule has no matches, defaulting to /", "httproute", key)
			add("/")
			continue
		}
		for _, match := range rule.Matches {
			if match.Path == nil || match.Path.Value == nil {
				log.Info("httproute match has no path, defaulting to /", "httproute", key)
				add("/")
				continue
			}
			if match.Path.Type != nil && *match.Path.Type == gwv1.PathMatchRegularExpression {
				log.Info("httproute match has regex path, cannot use as check URL, defaulting to /", "httproute", key, "path", *match.Path.Value)
				add("/")
				continue
			}
			p := *match.Path.Value
			if p == "" {
				log.Info("httproute match has empty path, defaulting to /", "httproute", key)
				p = "/"
			}
			add(p)
		}
	}
	if len(paths) == 0 {
		log.Info("httproute has no path matches, defaulting to /", "httproute", key)
		return []string{"/"}
	}
	return paths
}

func (r *IngressReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)

	seen := make(map[string]struct{})
	var endpoints []gatusEndpoint

	var ingressList networkingv1.IngressList
	if err := r.List(ctx, &ingressList); err != nil {
		return ctrl.Result{}, err
	}
	log.V(1).Info("found ingresses", "count", len(ingressList.Items))

	matches := ingressfilter.Filter(log, ingressList.Items, r.Selectors)
	log.V(1).Info("total ingress matches", "count", len(matches))

	for _, match := range matches {
		ing := match.Ingress
		var selectorChecks []config.CheckTemplate
		if match.Selector != nil {
			selectorChecks = match.Selector.Checks
		}
		checks := r.resolveChecks(selectorChecks)
		for _, rule := range ing.Spec.Rules {
			if rule.Host == "" {
				continue
			}
			ingKey := ing.Namespace + "/" + ing.Name
			paths := extractIngressPaths(log, rule, ingKey)
			for _, tmpl := range checks {
				effectivePaths := paths
				if tmpl.DNS != nil {
					effectivePaths = []string{"/"}
				}
				for _, path := range effectivePaths {
					name := fmt.Sprintf("%s/%s", ing.Namespace, ing.Name)
					if path != "/" {
						name += " " + path
					}
					if tmpl.NameSuffix != "" {
						name += " - " + tmpl.NameSuffix
					}
					ep := buildEndpoint(name, rule.Host, path, tmpl)
					epKey := endpointKey(ep)
					if _, ok := seen[epKey]; ok {
						log.V(1).Info("skipping duplicate endpoint", "name", name, "host", rule.Host)
						continue
					}
					seen[epKey] = struct{}{}
					log.V(1).Info("generating endpoint", "name", name, "url", ep.URL)
					endpoints = append(endpoints, ep)
				}
			}
		}
	}

	if r.WatchHTTPRoutes {
		var routeList gwv1.HTTPRouteList
		if err := r.List(ctx, &routeList); err != nil {
			return ctrl.Result{}, err
		}
		log.V(1).Info("found httproutes", "count", len(routeList.Items))

		routeMatches := httproutefilter.Filter(log, routeList.Items, r.HTTPRouteSelectors)
		log.V(1).Info("total httproute matches", "count", len(routeMatches))

		for _, match := range routeMatches {
			route := match.Route
			var selectorChecks []config.CheckTemplate
			if match.Selector != nil {
				selectorChecks = match.Selector.Checks
			}
			checks := r.resolveChecks(selectorChecks)
			routeKey := route.Namespace + "/" + route.Name
			paths := extractHTTPRoutePaths(log, route, routeKey)
			for _, hostname := range route.Spec.Hostnames {
				host := string(hostname)
				if host == "" {
					continue
				}
				for _, tmpl := range checks {
					effectivePaths := paths
					if tmpl.DNS != nil {
						effectivePaths = []string{"/"}
					}
					for _, path := range effectivePaths {
						name := fmt.Sprintf("%s/%s", route.Namespace, route.Name)
						if path != "/" {
							name += " " + path
						}
						if tmpl.NameSuffix != "" {
							name += " - " + tmpl.NameSuffix
						}
						ep := buildEndpoint(name, host, path, tmpl)
						epKey := endpointKey(ep)
						if _, ok := seen[epKey]; ok {
							log.V(1).Info("skipping duplicate endpoint", "name", name, "host", host)
							continue
						}
						seen[epKey] = struct{}{}
						log.V(1).Info("generating endpoint", "name", name, "url", ep.URL)
						endpoints = append(endpoints, ep)
					}
				}
			}
		}
	}

	if endpoints == nil {
		endpoints = []gatusEndpoint{}
	}

	dynamicYAML, err := yaml.Marshal(gatusConfig{Endpoints: endpoints})
	if err != nil {
		return ctrl.Result{}, err
	}

	newData := map[string]string{
		"dynamic.yaml": string(dynamicYAML),
	}
	if r.StaticConfig != nil {
		staticYAML, err := yaml.Marshal(r.StaticConfig)
		if err != nil {
			return ctrl.Result{}, err
		}
		newData["static.yaml"] = string(staticYAML)
	}

	existing := &corev1.ConfigMap{}
	err = r.Get(ctx, types.NamespacedName{Name: r.ConfigMapName, Namespace: r.ConfigMapNamespace}, existing)
	if errors.IsNotFound(err) {
		for _, ep := range endpoints {
			log.Info("endpoint added", "name", ep.Name, "url", ep.URL)
		}
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      r.ConfigMapName,
				Namespace: r.ConfigMapNamespace,
			},
			Data: newData,
		}
		log.Info("creating configmap")
		return ctrl.Result{}, r.Create(ctx, cm)
	}
	if err != nil {
		return ctrl.Result{}, err
	}

	if existing.Data["dynamic.yaml"] == newData["dynamic.yaml"] &&
		existing.Data["static.yaml"] == newData["static.yaml"] {
		log.V(1).Info("configmap unchanged, skipping update")
		return ctrl.Result{}, nil
	}

	// Diff old vs new endpoints and log changes at info level.
	var prevCfg gatusConfig
	_ = yaml.Unmarshal([]byte(existing.Data["dynamic.yaml"]), &prevCfg)
	prevByName := make(map[string]string, len(prevCfg.Endpoints))
	for _, ep := range prevCfg.Endpoints {
		prevByName[ep.Name] = endpointKey(ep)
	}
	newByName := make(map[string]struct{}, len(endpoints))
	for _, ep := range endpoints {
		newByName[ep.Name] = struct{}{}
		if prevKey, existed := prevByName[ep.Name]; !existed {
			log.Info("endpoint added", "name", ep.Name, "url", ep.URL)
		} else if prevKey != endpointKey(ep) {
			log.Info("endpoint updated", "name", ep.Name, "url", ep.URL)
		}
	}
	for _, ep := range prevCfg.Endpoints {
		if _, ok := newByName[ep.Name]; !ok {
			log.Info("endpoint removed", "name", ep.Name)
		}
	}

	existing.Data = newData
	log.Info("updating configmap")
	return ctrl.Result{}, r.Update(ctx, existing)
}

func (r *IngressReconciler) SetupWithManager(mgr ctrl.Manager) error {
	mapToConfigMap := handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
		return []reconcile.Request{{
			NamespacedName: types.NamespacedName{
				Name:      r.ConfigMapName,
				Namespace: r.ConfigMapNamespace,
			},
		}}
	})

	builder := ctrl.NewControllerManagedBy(mgr).
		Named("gatus-config").
		Watches(&networkingv1.Ingress{}, mapToConfigMap)

	if r.WatchHTTPRoutes {
		builder = builder.Watches(&gwv1.HTTPRoute{}, mapToConfigMap)
	}

	return builder.Complete(r)
}
