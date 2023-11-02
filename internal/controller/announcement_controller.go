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
	"github.com/CQUPTMirror/kubesync/api/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// AnnouncementReconciler reconciles a Announcement object
type AnnouncementReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=mirror.redrock.team,resources=announcements,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=mirror.redrock.team,resources=announcements/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=mirror.redrock.team,resources=announcements/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *AnnouncementReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)

	var news v1beta1.Announcement
	if err := r.Get(ctx, req.NamespacedName, &news); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if news.Status.PubTime == 0 {
		news.Status.PubTime = time.Now().Unix()
	}
	news.Status.EditTime = time.Now().Unix()

	err := r.Status().Update(ctx, &news)
	if err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *AnnouncementReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1beta1.Announcement{}).
		WithEventFilter(predicate.Funcs{
			UpdateFunc: func(e event.UpdateEvent) bool {
				oldGeneration := e.ObjectOld.GetGeneration()
				newGeneration := e.ObjectNew.GetGeneration()
				return oldGeneration != newGeneration
			},
		}).
		Complete(r)
}
