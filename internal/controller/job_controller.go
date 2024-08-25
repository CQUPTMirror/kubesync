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
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	mirrorv1beta1 "github.com/CQUPTMirror/kubesync/api/v1beta1"
)

// JobReconciler reconciles a Job object
type JobReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Config *Config
}

//+kubebuilder:rbac:groups=mirror.redrock.team,resources=jobs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=mirror.redrock.team,resources=jobs/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=mirror.redrock.team,resources=jobs/finalizers,verbs=update
//+kubebuilder:rbac:groups=core,resources=persistentvolumeclaims,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses,verbs=get;list;watch;create;update;patch;delete

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
	if job.Spec.Config.Type != "" && job.Spec.Config.Type != mirrorv1beta1.Mirror {
		return ctrl.Result{}, nil
	}

	var (
		err     error
		ig      *v1.Ingress
		frontCM *corev1.ConfigMap
		sm      *monitoringv1.ServiceMonitor
	)
	disableFront, _, _, _, _, _, _ := r.checkRsyncFront(&job)
	if !disableFront {
		ig, err = r.desiredIngress(&job)
		if err != nil {
			return ctrl.Result{}, err
		}
		frontCM, err = r.desiredFrontConfigmap(&job)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	pvc, err := r.desiredPersistentVolumeClaim(&job)
	if err != nil {
		return ctrl.Result{}, err
	}

	app, err := r.desiredDeployment(&job, managerName, frontCM)
	if err != nil {
		return ctrl.Result{}, err
	}

	svc, err := r.desiredService(&job)
	if err != nil {
		return ctrl.Result{}, err
	}

	if r.Config.EnableMetric {
		sm = r.desiredServiceMonitor(&job)
	}

	applyOpts := []client.PatchOption{client.ForceOwnership, client.FieldOwner("mirror-controller")}

	err = r.Patch(ctx, pvc, client.Apply, applyOpts...)
	if err != nil {
		return ctrl.Result{}, err
	}

	if app != nil {
		err = r.Patch(ctx, svc, client.Apply, applyOpts...)
		if err != nil {
			return ctrl.Result{}, err
		}

		if !disableFront {
			err = r.Patch(ctx, ig, client.Apply, applyOpts...)
			if err != nil {
				return ctrl.Result{}, err
			}
			if frontCM != nil {
				err = r.Patch(ctx, frontCM, client.Apply, applyOpts...)
				if err != nil {
					return ctrl.Result{}, err
				}
			}
		}
		err = r.Patch(ctx, app, client.Apply, applyOpts...)
		if err != nil {
			return ctrl.Result{}, err
		}

		if sm != nil {
			err = r.Patch(ctx, sm, client.Apply, applyOpts...)
			if err != nil {
				return ctrl.Result{}, err
			}
		}
	} else {
		deploy := new(appsv1.Deployment)
		err := r.Get(ctx, client.ObjectKey{Name: job.Name, Namespace: job.Namespace}, deploy)
		if err == nil || deploy != nil {
			r.Delete(ctx, &v1.Ingress{
				TypeMeta:   metav1.TypeMeta{APIVersion: v1.SchemeGroupVersion.String(), Kind: "Ingress"},
				ObjectMeta: metav1.ObjectMeta{Name: job.Name, Namespace: job.Namespace},
			})
			r.Delete(ctx, &corev1.Service{
				TypeMeta:   metav1.TypeMeta{APIVersion: corev1.SchemeGroupVersion.String(), Kind: "Service"},
				ObjectMeta: metav1.ObjectMeta{Name: job.Name, Namespace: job.Namespace},
			})
			r.Delete(ctx, deploy)
			r.Delete(ctx, &corev1.ConfigMap{
				TypeMeta:   metav1.TypeMeta{APIVersion: corev1.SchemeGroupVersion.String(), Kind: ConfigMapKind},
				ObjectMeta: metav1.ObjectMeta{Name: job.Name, Namespace: job.Namespace},
			})
			r.Delete(ctx, &monitoringv1.ServiceMonitor{
				TypeMeta:   metav1.TypeMeta{APIVersion: monitoringv1.SchemeGroupVersion.String(), Kind: "ServiceMonitor"},
				ObjectMeta: metav1.ObjectMeta{Name: job.Name, Namespace: job.Namespace},
			})
		}
	}

	if disableFront {
		ig := new(v1.Ingress)
		err := r.Get(ctx, client.ObjectKey{Name: job.Name, Namespace: job.Namespace}, ig)
		if err == nil || ig != nil {
			r.Delete(ctx, &v1.Ingress{
				TypeMeta:   metav1.TypeMeta{APIVersion: v1.SchemeGroupVersion.String(), Kind: "Ingress"},
				ObjectMeta: metav1.ObjectMeta{Name: job.Name, Namespace: job.Namespace},
			})
		}
	}

	if job.Status.Status == "" {
		job.Status.Status = mirrorv1beta1.Created
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
		WithEventFilter(predicate.Funcs{
			UpdateFunc: func(e event.UpdateEvent) bool {
				oldGeneration := e.ObjectOld.GetGeneration()
				newGeneration := e.ObjectNew.GetGeneration()
				return oldGeneration != newGeneration
			},
		}).
		Owns(&corev1.PersistentVolumeClaim{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Owns(&v1.Ingress{}).
		Complete(r)
}
