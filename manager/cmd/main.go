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
	"encoding/json"
	"flag"
	"github.com/CQUPTMirror/kubesync/manager/mirrorz"
	"os"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	mirrorv1beta1 "github.com/CQUPTMirror/kubesync/api/v1beta1"
	"github.com/CQUPTMirror/kubesync/manager"
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
	var apiAddr string
	addrEnv := os.Getenv("ADDR")
	if addrEnv == "" {
		addrEnv = ":3000"
	}
	flag.StringVar(&apiAddr, "addr", addrEnv, "The port the api endpoint binds to.")
	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	var mirrorZ *mirrorz.MirrorZ = nil
	var mirrorInfo mirrorz.MirrorZ
	if err := json.Unmarshal([]byte(os.Getenv("MIRRORZ")), &mirrorInfo); err == nil {
		mirrorZ = &mirrorInfo
	}

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	mgr, err := manager.GetTUNASyncManager(ctrl.GetConfigOrDie(), manager.Options{
		Scheme:  scheme,
		Address: apiAddr,
		MirrorZ: mirrorZ,
		Total:   os.Getenv("TOTAL"),
	})
	if err != nil {
		setupLog.Error(err, "unable to start api service")
		os.Exit(1)
	}

	setupLog.Info("starting api service")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running api service")
		os.Exit(1)
	}
}
