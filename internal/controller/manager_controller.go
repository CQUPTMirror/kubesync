/*
Copyright 2023 CQUPTMirror.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"errors"
	"github.com/hashicorp/go-multierror"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	mirrorv1beta1 "github.com/CQUPTMirror/kubesync/api/v1beta1"
)

// ManagerReconciler reconciles a Manager object
type ManagerReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=mirror.redrock.team,resources=managers,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=mirror.redrock.team,resources=managers/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=mirror.redrock.team,resources=managers/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *ManagerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)

	var manager mirrorv1beta1.Manager
	if err := r.Get(ctx, req.NamespacedName, &manager); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	var managerList mirrorv1beta1.ManagerList
	if err := r.List(ctx, &managerList, client.InNamespace(req.Namespace), client.MatchingFields{"status.phase": string(mirrorv1beta1.DeploySucceeded)}); err != nil {
		return ctrl.Result{}, err
	}
	if len(managerList.Items) > 0 {
		return ctrl.Result{}, errors.New("already have one active manager in this namespace")
	}

	app, err := r.desiredDeployment(manager)
	if err != nil {
		return ctrl.Result{}, err
	}

	svc, err := r.desiredService(manager)
	if err != nil {
		return ctrl.Result{}, err
	}

	applyOpts := []client.PatchOption{client.ForceOwnership, client.FieldOwner("mirror-controller")}

	err = r.Patch(ctx, &app, client.Apply, applyOpts...)
	if err != nil {
		return ctrl.Result{}, err
	}

	err = r.Patch(ctx, &svc, client.Apply, applyOpts...)
	if err != nil {
		e := r.Delete(ctx, &app)
		if e != nil {
			return ctrl.Result{}, multierror.Append(err, e)
		}
		return ctrl.Result{}, err
	}

	manager.Status.Phase = mirrorv1beta1.DeploySucceeded

	err = r.Status().Update(ctx, &manager)
	if err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ManagerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&mirrorv1beta1.Manager{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Complete(r)
}
