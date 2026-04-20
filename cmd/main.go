package main

import (
	"flag"
	"fmt"
	"os"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/belitre/gatus-config-controller/internal/config"
	"github.com/belitre/gatus-config-controller/internal/controller"
)

var (
	Version = "canary"
	Commit  = "unknown"
	Date    = "unknown"
)

var scheme = runtime.NewScheme()

func init() {
	_ = clientgoscheme.AddToScheme(scheme)
	_ = gwv1.Install(scheme)
}

func main() {
	var configMapNamespace string
	var configMapName string
	var configPath string
	var leaderElect bool
	var leaderElectionID string
	var leaderElectionResourceLock string
	var watchHTTPRoutes bool

	flag.StringVar(&configMapNamespace, "configmap-namespace", "default", "namespace of the Gatus dynamic config ConfigMap")
	flag.StringVar(&configMapName, "configmap-name", "gatus-dynamic-config", "name of the Gatus dynamic config ConfigMap")
	flag.StringVar(&configPath, "config", "", "path to controller config file (optional)")
	flag.BoolVar(&leaderElect, "leader-elect", false, "enable leader election for HA deployments")
	flag.StringVar(&leaderElectionID, "leader-election-id", "gatus-config-controller", "resource name used for leader election")
	flag.StringVar(&leaderElectionResourceLock, "leader-election-resource-lock", "leases", "resource type used for leader election: leases (default) or configmaps")
	flag.BoolVar(&watchHTTPRoutes, "watch-httproutes", false, "enable watching HTTPRoute resources (gateway.networking.k8s.io CRD must be installed)")

	opts := zap.Options{}
	opts.BindFlags(flag.CommandLine)

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "gatus-config-controller version %s (commit: %s, built: %s)\n\n", Version, Commit, Date)
		flag.PrintDefaults()
	}
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))
	ctrl.Log.Info("starting", "version", Version, "commit", Commit, "date", Date)

	var selectors []config.IngressSelector
	var defaultChecks []config.CheckTemplate
	var httpRouteSelectors []config.HTTPRouteSelector
	var staticConfig any
	if configPath != "" {
		cfg, err := config.Load(configPath)
		if err != nil {
			ctrl.Log.Error(err, "unable to load config", "path", configPath)
			os.Exit(1)
		}
		selectors = cfg.Ingresses
		defaultChecks = cfg.DefaultChecks
		httpRouteSelectors = cfg.HTTPRoutes
		staticConfig = cfg.StaticConfig
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
		Cache: cache.Options{
			ByObject: map[client.Object]cache.ByObject{
				&corev1.ConfigMap{}: {
					Namespaces: map[string]cache.Config{
						configMapNamespace: {},
					},
				},
			},
		},
		LeaderElection:             leaderElect,
		LeaderElectionID:           leaderElectionID,
		LeaderElectionResourceLock: leaderElectionResourceLock,
	})
	if err != nil {
		ctrl.Log.Error(err, "unable to start manager")
		os.Exit(1)
	}

	if watchHTTPRoutes {
		httpRouteGVR := schema.GroupVersionResource{
			Group:    "gateway.networking.k8s.io",
			Version:  "v1",
			Resource: "httproutes",
		}
		if _, err := mgr.GetRESTMapper().ResourceFor(httpRouteGVR); err != nil {
			ctrl.Log.Error(err, "HTTPRoute CRD not available but --watch-httproutes is enabled")
			os.Exit(1)
		}
	}

	if err := (&controller.IngressReconciler{
		Client:             mgr.GetClient(),
		ConfigMapNamespace: configMapNamespace,
		ConfigMapName:      configMapName,
		Selectors:          selectors,
		DefaultChecks:      defaultChecks,
		WatchHTTPRoutes:    watchHTTPRoutes,
		HTTPRouteSelectors: httpRouteSelectors,
		StaticConfig:       staticConfig,
	}).SetupWithManager(mgr); err != nil {
		ctrl.Log.Error(err, "unable to create ingress controller")
		os.Exit(1)
	}

	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		ctrl.Log.Error(err, "problem running manager")
		os.Exit(1)
	}
}
