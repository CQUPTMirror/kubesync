/*
Copyright (C) 2023  CQUPTMirror

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU General Public License for more details.

You should have received a copy of the GNU General Public License
along with this program.  If not, see <https://www.gnu.org/licenses/>.
*/

package main

import (
	"flag"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	"log"
	"os"
	"reflect"
	"strings"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	mirrorv1beta1 "github.com/CQUPTMirror/kubesync/api/v1beta1"
	"github.com/CQUPTMirror/kubesync/internal/controller"
	//+kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(mirrorv1beta1.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	var probeAddr string
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		Metrics:                metricsserver.Options{BindAddress: metricsAddr},
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "fbcb7ae1.redrock.team",
		// LeaderElectionReleaseOnCancel defines if the leader should step down voluntarily
		// when the Manager ends. This requires the binary to immediately end when the
		// Manager is stopped, otherwise, this setting is unsafe. Setting this significantly
		// speeds up voluntary leader transitions as the new leader don't have to wait
		// LeaseDuration time first.
		//
		// In the default scaffold provided, the program ends immediately after
		// the manager stops, so would be fine to enable this option. However,
		// if you are doing or is intended to do any operation such as perform cleanups
		// after the manager stops then its usage might be unsafe.
		// LeaderElectionReleaseOnCancel: true,
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	config := getConfig()

	// Register the ServiceMonitor API
	if err := monitoringv1.AddToScheme(mgr.GetScheme()); err != nil {
		log.Fatalf("unable to register ServiceMonitor API: %v", err)
	}

	if err = (&controller.JobReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
		Config: &config,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Job")
		os.Exit(1)
	}
	if err = (&controller.ManagerReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
		Config: &config,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Manager")
		os.Exit(1)
	}
	if err = (&controller.AnnouncementReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Announcement")
		os.Exit(1)
	}
	//+kubebuilder:scaffold:builder

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}

func getConfig() controller.Config {
	// TODO: replace ugly env way to config file
	annString := os.Getenv("FRONT_ANN")
	annItems := make(map[string]string)
	if annString != "" {
		for _, item := range strings.Split(annString, ";") {
			splits := strings.Split(item, "=")
			if len(splits) == 2 {
				annItems[splits[0]] = splits[1]
			}
		}
	}

	debug := false
	if os.Getenv("DEBUG") != "" {
		debug = true
	}

	enableMetric := false
	if os.Getenv("ENABLE_METRIC") != "" {
		enableMetric = true
	}

	c := controller.Config{
		ManagerImage: os.Getenv("MANAGER_IMAGE"),
		WorkerImage:  os.Getenv("WORKER_IMAGE"),
		PullPolicy:   os.Getenv("PULL_POLICY"),
		PullSecret:   os.Getenv("PULL_SECRET"),
		StorageClass: os.Getenv("STORAGE_CLASS"),
		AccessMode:   os.Getenv("ACCESS_MODE"),
		FrontMode:    os.Getenv("FRONT_MODE"),
		FrontImage:   os.Getenv("FRONT_IMAGE"),
		RsyncImage:   os.Getenv("RSYNC_IMAGE"),
		FrontCmd:     os.Getenv("FRONT_CMD"),
		FrontConfig:  os.Getenv("FRONT_CONFIG"),
		RsyncCmd:     os.Getenv("RSYNC_CMD"),
		FrontHost:    os.Getenv("FRONT_HOST"),
		FrontTLS:     os.Getenv("FRONT_TLS"),
		FrontClass:   os.Getenv("FRONT_CLASS"),
		FrontAnn:     annItems,
		EnableMetric: enableMetric,
		Debug:        debug,
	}

	mergeDefaults(&c)

	return c
}

// mergeDefaults merges the default values into the given config.
// only support string and map[string]string.
func mergeDefaults(config *controller.Config) {
	defaultConfig := &controller.Config{
		ManagerImage: "cquptmirror/manager:latest",
		WorkerImage:  "cquptmirror/worker:latest",
		FrontMode:    "cquptmirror/caddy:latest",
		RsyncImage:   "",
		FrontCmd:     "",
		FrontConfig: `
{
    "logging": {
        "logs": {
            "default": {},
            "loki": {
                "writer": {
                    "labels": {
                        "job": "{env.JOB_NAME}",
                        "instance": "{env.HOSTNAME}",
                        "component": "caddy"
                    },
                    "output": "loki",
                    "url": "http://loki:3100/loki/api/v1/push"
                },
                "encoder": {
                    "format": "json"
                },
                "level": "INFO",
                "include": [
                    "http.log.access"
                ]
            }
        }
    },
    "apps": {
        "http": {
            "servers": {
                "metric": {
                    "listen": [
                        ":2019"
                    ],
                    "routes": [
                        {
                            "handle": [
                                {
                                    "handler": "metrics"
                                }
                            ]
                        }
                    ]
                },
                "file_server": {
                    "listen": [
                        ":80"
                    ],
                    "routes": [
                        {
                            "handle": [
                                {
                                    "browse": {},
                                    "handler": "file_server",
                                    "root": "/data"
                                }
                            ]
                        }
                    ],
                    "logs": {
                        "default_logger_name": "loki"
                    },
                    "metrics": {}
                }
            }
        }
    }
}
		`,
		RsyncCmd:   "",
		FrontHost:  "mirrors.cqupt.edu.cn",
		FrontTLS:   "",
		FrontClass: "traefik",
		FrontAnn: map[string]string{
			"traefik.ingress.kubernetes.io/router.entrypoints": "web",
		},
		EnableMetric: false,
		Debug:        false,
	}

	// 用反射实现
	configValue := reflect.ValueOf(config).Elem()
	defaultConfigValue := reflect.ValueOf(defaultConfig).Elem()

	for i := 0; i < configValue.NumField(); i++ {
		configField := configValue.Field(i)
		defaultConfigField := defaultConfigValue.Field(i)

		switch configField.Kind() {
		case reflect.String:
			if configField.IsZero() {
				configField.SetString(defaultConfigField.String())
			}
		case reflect.Map:
			// only support map[string]string
			if configField.IsZero() {
				// 新建一个map,防止defaultConfig依然被引用而无法被gc
				newMap := make(map[string]string)
				for _, key := range defaultConfigField.MapKeys() {
					newMap[key.String()] = defaultConfigField.MapIndex(key).String()
				}
				configField.Set(reflect.ValueOf(newMap))
			}
		default:
			continue
		}
	}
}
