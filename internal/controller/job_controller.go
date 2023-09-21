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
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	mirrorv1beta1 "github.com/CQUPTMirror/kubesync/api/v1beta1"
)

type Config struct {
	FrontImage string
	RsyncImage string
	FrontHost  string
	FrontTLS   string
	FrontClass string
	FrontAnn   map[string]string
}

// JobReconciler reconciles a Job object
type JobReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Config *Config
}

//+kubebuilder:rbac:groups=mirror.redrock.team,resources=jobs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=mirror.redrock.team,resources=jobs/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=mirror.redrock.team,resources=jobs/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *JobReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)

	var job mirrorv1beta1.Job
	if err := r.Get(ctx, req.NamespacedName, &job); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	var managerName string
	var managerList mirrorv1beta1.ManagerList
	if err := r.List(ctx, &managerList, client.InNamespace(req.Namespace), client.MatchingFields{"status.phase": string(mirrorv1beta1.DeploySucceeded)}); err != nil {
		return ctrl.Result{}, err
	}
	if len(managerList.Items) < 1 {
		return ctrl.Result{}, errors.New("no active manager in this namespace")
	} else {
		managerName = managerList.Items[0].Name
	}

	pvc, err := r.desiredPersistentVolumeClaim(job)
	if err != nil {
		return ctrl.Result{}, err
	}

	app, err := r.desiredDeployment(job, managerName)
	if err != nil {
		return ctrl.Result{}, err
	}

	svc, err := r.desiredService(job)
	if err != nil {
		return ctrl.Result{}, err
	}

	var ig v1.Ingress
	disableFront, _, _, _ := r.checkRsyncFront(&job)
	if !disableFront {
		ig, err = r.desiredIngress(job)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	applyOpts := []client.PatchOption{client.ForceOwnership, client.FieldOwner("mirror-controller")}

	err = r.Patch(ctx, &pvc, client.Apply, applyOpts...)
	if err != nil {
		return ctrl.Result{}, err
	}

	err = r.Patch(ctx, &app, client.Apply, applyOpts...)
	if err != nil {
		return ctrl.Result{}, err
	}

	err = r.Patch(ctx, &svc, client.Apply, applyOpts...)
	if err != nil {
		return ctrl.Result{}, err
	}

	if !disableFront {
		err = r.Patch(ctx, &ig, client.Apply, applyOpts...)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	err = r.Status().Update(ctx, &job)
	if err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *JobReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&mirrorv1beta1.Job{}).
		Owns(&corev1.PersistentVolumeClaim{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Owns(&v1.Ingress{}).
		Complete(r)
}
