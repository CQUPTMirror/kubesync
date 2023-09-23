package controller

import (
	"fmt"
	"github.com/CQUPTMirror/kubesync/api/v1beta1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
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
		Rules: []v1.PolicyRule{{
			APIGroups: []string{v1beta1.GroupVersion.String()}, Resources: []string{"jobs"},
			Verbs: []string{"create", "delete", "get", "list", "patch", "update", "watch"},
		}},
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
