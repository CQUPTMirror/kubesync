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
	"fmt"
	"github.com/CQUPTMirror/kubesync/api/v1beta1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	v12 "k8s.io/api/networking/v1"
	v1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
)

const ManagerPort = 3000

func (r *ManagerReconciler) desiredSA(manager *v1beta1.Manager) (*corev1.ServiceAccount, error) {
	sa := corev1.ServiceAccount{
		TypeMeta: metav1.TypeMeta{APIVersion: corev1.SchemeGroupVersion.String(), Kind: "ServiceAccount"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      manager.Name + "-sa",
			Namespace: manager.Namespace,
			Labels:    map[string]string{"manager": manager.Name},
		},
	}

	if err := ctrl.SetControllerReference(manager, &sa, r.Scheme); err != nil {
		return &sa, err
	}
	return &sa, nil
}

func (r *ManagerReconciler) desiredRole(manager *v1beta1.Manager) (*v1.Role, error) {
	role := v1.Role{
		TypeMeta: metav1.TypeMeta{APIVersion: v1.SchemeGroupVersion.String(), Kind: "Role"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      manager.Name + "-role",
			Namespace: manager.Namespace,
			Labels:    map[string]string{"manager": manager.Name},
		},
		Rules: []v1.PolicyRule{
			{
				APIGroups: []string{v1beta1.GroupVersion.Group}, Resources: []string{"jobs"},
				Verbs: []string{"create", "delete", "get", "list", "patch", "update", "watch"},
			},
			{
				APIGroups: []string{v1beta1.GroupVersion.Group}, Resources: []string{"jobs/status"},
				Verbs: []string{"get", "patch", "update"},
			},
			{
				APIGroups: []string{v1beta1.GroupVersion.Group}, Resources: []string{"announcements"},
				Verbs: []string{"create", "delete", "get", "list", "patch", "update", "watch"},
			},
			{
				APIGroups: []string{v1beta1.GroupVersion.Group}, Resources: []string{"announcements/status"},
				Verbs: []string{"get", "patch", "update"},
			},
			{
				APIGroups: []string{v1beta1.GroupVersion.Group}, Resources: []string{"files"},
				Verbs: []string{"create", "delete", "get", "list", "patch", "update", "watch"},
			},
			{
				APIGroups: []string{v1beta1.GroupVersion.Group}, Resources: []string{"files/status"},
				Verbs: []string{"get", "patch", "update"},
			},
		},
	}

	if err := ctrl.SetControllerReference(manager, &role, r.Scheme); err != nil {
		return &role, err
	}
	return &role, nil
}

func (r *ManagerReconciler) desiredRoleBinding(manager *v1beta1.Manager) (*v1.RoleBinding, error) {
	roleBinding := v1.RoleBinding{
		TypeMeta: metav1.TypeMeta{APIVersion: v1.SchemeGroupVersion.String(), Kind: "RoleBinding"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      manager.Name,
			Namespace: manager.Namespace,
			Labels:    map[string]string{"manager": manager.Name},
		},
		Subjects: []v1.Subject{{Kind: v1.ServiceAccountKind, Name: manager.Name + "-sa"}},
		RoleRef:  v1.RoleRef{APIGroup: v1.SchemeGroupVersion.Group, Kind: "Role", Name: manager.Name + "-role"},
	}

	if err := ctrl.SetControllerReference(manager, &roleBinding, r.Scheme); err != nil {
		return &roleBinding, err
	}
	return &roleBinding, nil
}

func (r *ManagerReconciler) desiredDeployment(manager *v1beta1.Manager) (*appsv1.Deployment, error) {
	probe := &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			TCPSocket: &corev1.TCPSocketAction{Port: intstr.FromInt(ManagerPort)},
		},
		InitialDelaySeconds: 10,
		TimeoutSeconds:      5,
		PeriodSeconds:       30,
		SuccessThreshold:    1,
		FailureThreshold:    5,
	}
	app := appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{APIVersion: appsv1.SchemeGroupVersion.String(), Kind: "Deployment"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      manager.Name,
			Namespace: manager.Namespace,
			Labels:    map[string]string{"manager": manager.Name},
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"manager": manager.Name},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"manager": manager.Name},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  manager.Name,
							Image: manager.Spec.Deploy.Image,
							Env: []corev1.EnvVar{
								{Name: "NAMESPACE", ValueFrom: &corev1.EnvVarSource{FieldRef: &corev1.ObjectFieldSelector{FieldPath: "metadata.namespace"}}},
								{Name: "ADDR", Value: fmt.Sprintf(":%d", ManagerPort)},
							},
							LivenessProbe:  probe,
							ReadinessProbe: probe,
							Ports: []corev1.ContainerPort{
								{ContainerPort: ManagerPort, Name: "api", Protocol: "TCP"},
							},
						},
					},
					ServiceAccountName: manager.Name + "-sa",
				},
			},
		},
	}
	if manager.Spec.Deploy.ImagePullPolicy != "" {
		app.Spec.Template.Spec.Containers[0].ImagePullPolicy = manager.Spec.Deploy.ImagePullPolicy
	}
	if manager.Spec.Deploy.MemoryLimit != "" || manager.Spec.Deploy.CPULimit != "" {
		app.Spec.Template.Spec.Containers[0].Resources = corev1.ResourceRequirements{Limits: corev1.ResourceList{}}
		if manager.Spec.Deploy.MemoryLimit != "" {
			app.Spec.Template.Spec.Containers[0].Resources.Limits[corev1.ResourceLimitsMemory] = resource.MustParse(manager.Spec.Deploy.MemoryLimit)
		}
		if manager.Spec.Deploy.CPULimit != "" {
			app.Spec.Template.Spec.Containers[0].Resources.Limits[corev1.ResourceLimitsCPU] = resource.MustParse(manager.Spec.Deploy.CPULimit)
		}
	}
	if manager.Spec.Deploy.ImagePullSecrets != nil {
		app.Spec.Template.Spec.ImagePullSecrets = manager.Spec.Deploy.ImagePullSecrets
	}
	if manager.Spec.Deploy.NodeName != "" {
		app.Spec.Template.Spec.NodeName = manager.Spec.Deploy.NodeName
	}
	if manager.Spec.Deploy.Affinity != nil {
		app.Spec.Template.Spec.Affinity = manager.Spec.Deploy.Affinity
	}
	if manager.Spec.Deploy.Tolerations != nil {
		app.Spec.Template.Spec.Tolerations = manager.Spec.Deploy.Tolerations
	}

	if err := ctrl.SetControllerReference(manager, &app, r.Scheme); err != nil {
		return &app, err
	}
	return &app, nil
}

func (r *ManagerReconciler) desiredService(manager *v1beta1.Manager) (*corev1.Service, error) {
	svc := corev1.Service{
		TypeMeta: metav1.TypeMeta{APIVersion: corev1.SchemeGroupVersion.String(), Kind: "Service"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      manager.Name,
			Namespace: manager.Namespace,
			Labels:    map[string]string{"manager": manager.Name},
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{Name: "api", Port: ManagerPort, Protocol: "TCP", TargetPort: intstr.FromString("api")},
			},
			Selector: map[string]string{"manager": manager.Name},
			Type:     corev1.ServiceTypeClusterIP,
		},
	}

	if err := ctrl.SetControllerReference(manager, &svc, r.Scheme); err != nil {
		return &svc, err
	}
	return &svc, nil
}

func (r *ManagerReconciler) desiredIngress(manager *v1beta1.Manager) (*v12.Ingress, error) {
	annotations := make(map[string]string)
	for k, v := range r.Config.FrontAnn {
		annotations[k] = v
	}
	for k, v := range manager.Spec.Ingress.Annotations {
		annotations[k] = v
	}

	pathType := v12.PathTypeExact
	svc := v12.IngressBackend{
		Service: &v12.IngressServiceBackend{
			Name: manager.Name,
			Port: v12.ServiceBackendPort{Name: "api"},
		},
	}

	ig := v12.Ingress{
		TypeMeta: metav1.TypeMeta{APIVersion: v12.SchemeGroupVersion.String(), Kind: "Ingress"},
		ObjectMeta: metav1.ObjectMeta{
			Name:        manager.Name,
			Namespace:   manager.Namespace,
			Labels:      map[string]string{"manager": manager.Name},
			Annotations: annotations,
		},
		Spec: v12.IngressSpec{
			Rules: []v12.IngressRule{
				{
					IngressRuleValue: v12.IngressRuleValue{
						HTTP: &v12.HTTPIngressRuleValue{
							Paths: []v12.HTTPIngressPath{
								{Path: "/api/mirrors", PathType: &pathType, Backend: svc},
								{Path: "/api/news", PathType: &pathType, Backend: svc},
								{Path: "/api/files", PathType: &pathType, Backend: svc},
							},
						},
					},
				},
			},
		},
	}

	if r.Config.FrontClass != "" || manager.Spec.Ingress.IngressClass != "" {
		ig.Spec.IngressClassName = &r.Config.FrontClass
		if manager.Spec.Ingress.IngressClass != "" {
			ig.Spec.IngressClassName = &manager.Spec.Ingress.IngressClass
		}
	}

	if r.Config.FrontTLS != "" || manager.Spec.Ingress.TLSSecret != "" {
		secretName := r.Config.FrontTLS
		if manager.Spec.Ingress.TLSSecret != "" {
			secretName = manager.Spec.Ingress.TLSSecret
		}
		ig.Spec.TLS = []v12.IngressTLS{{SecretName: secretName}}
	}

	if r.Config.FrontHost != "" || manager.Spec.Ingress.Host != "" {
		ig.Spec.Rules[0].Host = r.Config.FrontHost
		if manager.Spec.Ingress.Host != "" {
			ig.Spec.Rules[0].Host = manager.Spec.Ingress.Host
		}
	}

	if err := ctrl.SetControllerReference(manager, &ig, r.Scheme); err != nil {
		return &ig, err
	}
	return &ig, nil
}
