package main

import (
	"flag"
	"fmt"
	"os"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

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
}

func main() {
	var configMapNamespace string
	var configMapName string
	var configPath string

	flag.StringVar(&configMapNamespace, "configmap-namespace", "default", "namespace of the Gatus dynamic config ConfigMap")
	flag.StringVar(&configMapName, "configmap-name", "gatus-dynamic-config", "name of the Gatus dynamic config ConfigMap")
	flag.StringVar(&configPath, "config", "", "path to controller config file (optional)")

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
	if configPath != "" {
		cfg, err := config.Load(configPath)
		if err != nil {
			ctrl.Log.Error(err, "unable to load config", "path", configPath)
			os.Exit(1)
		}
		selectors = cfg.Ingresses
		defaultChecks = cfg.DefaultChecks
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
	})
	if err != nil {
		ctrl.Log.Error(err, "unable to start manager")
		os.Exit(1)
	}

	if err := (&controller.IngressReconciler{
		Client:             mgr.GetClient(),
		ConfigMapNamespace: configMapNamespace,
		ConfigMapName:      configMapName,
		Selectors:          selectors,
		DefaultChecks:      defaultChecks,
	}).SetupWithManager(mgr); err != nil {
		ctrl.Log.Error(err, "unable to create ingress controller")
		os.Exit(1)
	}

	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		ctrl.Log.Error(err, "problem running manager")
		os.Exit(1)
	}
}
