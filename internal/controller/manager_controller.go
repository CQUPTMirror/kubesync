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

package controller

import (
	"context"
	"errors"
	mirrorv1beta1 "github.com/CQUPTMirror/kubesync/api/v1beta1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	v12 "k8s.io/api/networking/v1"
	v1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

type Config struct {
	ManagerImage string
	WorkerImage  string
	PullPolicy   string
	PullSecret   string
	StorageClass string
	AccessMode   string
	FrontMode    string
	FrontImage   string
	RsyncImage   string
	FrontCmd     string
	FrontConfig  string
	RsyncCmd     string
	FrontHost    string
	FrontTLS     string
	FrontClass   string
	FrontAnn     map[string]string
	Debug        bool
}

// ManagerReconciler reconciles a Manager object
type ManagerReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Config *Config
}

// +kubebuilder:rbac:groups=mirror.redrock.team,resources=managers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=mirror.redrock.team,resources=managers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=mirror.redrock.team,resources=managers/finalizers,verbs=update
// +kubebuilder:rbac:groups=core,resources=serviceaccounts,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=roles,verbs=get;list;watch;create;update;patch;delete;escalate;bind
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=rolebindings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=daemonsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=mirror.redrock.team,resources=files,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=mirror.redrock.team,resources=files/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=mirror.redrock.team,resources=files/finalizers,verbs=update

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
	if len(managerList.Items) > 0 && managerList.Items[0].Name != manager.Name {
		return ctrl.Result{}, errors.New("already have one active manager in this namespace")
	}

	sa, err := r.desiredSA(&manager)
	if err != nil {
		return ctrl.Result{}, err
	}

	role, err := r.desiredRole(&manager)
	if err != nil {
		return ctrl.Result{}, err
	}

	rb, err := r.desiredRoleBinding(&manager)
	if err != nil {
		return ctrl.Result{}, err
	}

	app, err := r.desiredDeployment(&manager)
	if err != nil {
		return ctrl.Result{}, err
	}

	svc, err := r.desiredService(&manager)
	if err != nil {
		return ctrl.Result{}, err
	}

	ig, err := r.desiredIngress(&manager)
	if err != nil {
		return ctrl.Result{}, err
	}

	applyOpts := []client.PatchOption{client.ForceOwnership, client.FieldOwner("mirror-controller")}

	err = r.Patch(ctx, sa, client.Apply, applyOpts...)
	if err != nil {
		return ctrl.Result{}, err
	}

	err = r.Patch(ctx, role, client.Apply, applyOpts...)
	if err != nil {
		return ctrl.Result{}, err
	}

	err = r.Patch(ctx, rb, client.Apply, applyOpts...)
	if err != nil {
		return ctrl.Result{}, err
	}

	switch app := app.(type) {
	case *appsv1.Deployment:
		ds := new(appsv1.DaemonSet)
		if err := r.Get(ctx, client.ObjectKey{Name: manager.Name, Namespace: manager.Namespace}, ds); err == nil || ds != nil {
			r.Delete(ctx, ds)
		}
		err = r.Patch(ctx, app, client.Apply, applyOpts...)
	case *appsv1.DaemonSet:
		dm := new(appsv1.Deployment)
		if err := r.Get(ctx, client.ObjectKey{Name: manager.Name, Namespace: manager.Namespace}, dm); err == nil || dm != nil {
			r.Delete(ctx, dm)
		}
		err = r.Patch(ctx, app, client.Apply, applyOpts...)
	default:
		return ctrl.Result{}, err
	}

	if err != nil {
		return ctrl.Result{}, err
	}

	err = r.Patch(ctx, svc, client.Apply, applyOpts...)
	if err != nil {
		return ctrl.Result{}, err
	}

	err = r.Patch(ctx, ig, client.Apply, applyOpts...)
	if err != nil {
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
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &mirrorv1beta1.Manager{}, "status.phase", func(rawObj client.Object) []string {
		manager := rawObj.(*mirrorv1beta1.Manager)
		return []string{string(manager.Status.Phase)}
	}); err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&mirrorv1beta1.Manager{}).
		WithEventFilter(predicate.Funcs{
			UpdateFunc: func(e event.UpdateEvent) bool {
				oldGeneration := e.ObjectOld.GetGeneration()
				newGeneration := e.ObjectNew.GetGeneration()
				return oldGeneration != newGeneration
			},
		}).
		Owns(&corev1.ServiceAccount{}).
		Owns(&v1.Role{}).
		Owns(&v1.RoleBinding{}).
		Owns(&appsv1.DaemonSet{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Owns(&v12.Ingress{}).
		Complete(r)
}
