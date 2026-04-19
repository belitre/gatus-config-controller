package controller

import (
	"context"
	"fmt"

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

	"github.com/belitre/gatus-config-controller/internal/config"
	ingressfilter "github.com/belitre/gatus-config-controller/internal/ingress"
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
}

type gatusClient struct {
	IgnoreRedirect bool `yaml:"ignore-redirect,omitempty"`
}

type gatusEndpoint struct {
	Name       string       `yaml:"name"`
	URL        string       `yaml:"url"`
	Interval   string       `yaml:"interval"`
	Conditions []string     `yaml:"conditions"`
	Client     *gatusClient `yaml:"client,omitempty"`
}

type gatusConfig struct {
	Endpoints []gatusEndpoint `yaml:"endpoints"`
}

func (r *IngressReconciler) resolveChecks(match ingressfilter.MatchResult) []config.CheckTemplate {
	if match.Selector != nil && len(match.Selector.Checks) > 0 {
		return match.Selector.Checks
	}
	if len(r.DefaultChecks) > 0 {
		return r.DefaultChecks
	}
	return builtinChecks
}

func endpointKey(ep gatusEndpoint) string {
	return fmt.Sprintf("%s|%s|%s|%v", ep.Name, ep.URL, ep.Interval, ep.Conditions)
}

func (r *IngressReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)

	var ingressList networkingv1.IngressList
	if err := r.List(ctx, &ingressList); err != nil {
		return ctrl.Result{}, err
	}
	log.V(1).Info("found ingresses", "count", len(ingressList.Items))

	matches := ingressfilter.Filter(log, ingressList.Items, r.Selectors)
	log.V(1).Info("total matches", "count", len(matches))

	seen := make(map[string]struct{})
	var endpoints []gatusEndpoint
	for _, match := range matches {
		ing := match.Ingress
		checks := r.resolveChecks(match)
		for _, rule := range ing.Spec.Rules {
			if rule.Host == "" {
				continue
			}
			for _, tmpl := range checks {
				name := fmt.Sprintf("%s/%s", ing.Namespace, ing.Name)
				if tmpl.NameSuffix != "" {
					name += " - " + tmpl.NameSuffix
				}
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
					URL:        tmpl.Scheme + "://" + rule.Host,
					Interval:   interval,
					Conditions: conditions,
				}
				if tmpl.NoFollowRedirects {
					ep.Client = &gatusClient{IgnoreRedirect: true}
				}
				key := endpointKey(ep)
				if _, ok := seen[key]; ok {
					log.V(1).Info("skipping duplicate endpoint", "name", name, "host", rule.Host)
					continue
				}
				seen[key] = struct{}{}
				log.V(1).Info("generating endpoint", "name", name, "url", ep.URL)
				endpoints = append(endpoints, ep)
			}
		}
	}

	if endpoints == nil {
		endpoints = []gatusEndpoint{}
	}

	data, err := yaml.Marshal(gatusConfig{Endpoints: endpoints})
	if err != nil {
		return ctrl.Result{}, err
	}

	existing := &corev1.ConfigMap{}
	err = r.Get(ctx, types.NamespacedName{Name: r.ConfigMapName, Namespace: r.ConfigMapNamespace}, existing)
	if errors.IsNotFound(err) {
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      r.ConfigMapName,
				Namespace: r.ConfigMapNamespace,
			},
			Data: map[string]string{"dynamic.yaml": string(data)},
		}
		log.Info("creating configmap")
		return ctrl.Result{}, r.Create(ctx, cm)
	}
	if err != nil {
		return ctrl.Result{}, err
	}

	if existing.Data == nil {
		existing.Data = map[string]string{}
	}
	existing.Data["dynamic.yaml"] = string(data)
	log.Info("updating configmap")
	return ctrl.Result{}, r.Update(ctx, existing)
}

func (r *IngressReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		Named("ingress-gatus-config").
		Watches(
			&networkingv1.Ingress{},
			handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
				return []reconcile.Request{{
					NamespacedName: types.NamespacedName{
						Name:      r.ConfigMapName,
						Namespace: r.ConfigMapNamespace,
					},
				}}
			}),
		).
		Complete(r)
}
