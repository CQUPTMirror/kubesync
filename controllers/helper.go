package controllers

import (
	"strconv"

	jobsv1beta1 "github.com/ztelliot/kubesync/api/v1beta1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	extensionsv1 "k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
)

func (r *JobReconciler) desiredConfigMap(job jobsv1beta1.Job) (corev1.ConfigMap, error) {
	cm := corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{APIVersion: corev1.SchemeGroupVersion.String(), Kind: "ConfigMap"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      job.Name,
			Namespace: job.Namespace,
			Labels:    map[string]string{"job": job.Name},
		},
		Data: map[string]string{
			"UPSTREAM":        job.Spec.Config.Upstream,
			"PROVIDER":        job.Spec.Config.Provider,
			"COMMAND":         job.Spec.Config.Command,
			"CONCURRENT":      strconv.Itoa(job.Spec.Config.Concurrent),
			"INTERVAL":        strconv.Itoa(job.Spec.Config.Interval),
			"RSYNCOPTIONS":    job.Spec.Config.RsyncOptions,
			"SIZEPATTERN":     job.Spec.Config.SizePattern,
			"ADDITIONOPTIONS": job.Spec.Config.AdditionOptions,
		},
	}

	if err := ctrl.SetControllerReference(&job, &cm, r.Scheme); err != nil {
		return cm, err
	}
	return cm, nil
}

func (r *JobReconciler) desiredPersistentVolumeClaim(job jobsv1beta1.Job) (corev1.PersistentVolumeClaim, error) {
	resourceStorage, _ := resource.ParseQuantity(job.Spec.Volume.Size)
	pvc := corev1.PersistentVolumeClaim{
		TypeMeta: metav1.TypeMeta{APIVersion: corev1.SchemeGroupVersion.String(), Kind: "PersistentVolumeClaim"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      job.Name,
			Namespace: job.Namespace,
			Labels:    map[string]string{"job": job.Name},
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{corev1.ResourceStorage: resourceStorage},
			},
		},
	}
	if job.Spec.Volume.AccessModes != nil {
		pvc.Spec.AccessModes = job.Spec.Volume.AccessModes
	}
	if job.Spec.Volume.StorageClassName != nil {
		pvc.Spec.StorageClassName = job.Spec.Volume.StorageClassName
	}

	if err := ctrl.SetControllerReference(&job, &pvc, r.Scheme); err != nil {
		return pvc, err
	}
	return pvc, nil
}

func (r *JobReconciler) desiredDeployment(job jobsv1beta1.Job) (appsv1.Deployment, error) {
	limitMemory, _ := resource.ParseQuantity(job.Spec.Deploy.MemoryLimit)
	limitCPU, _ := resource.ParseQuantity(job.Spec.Deploy.CPULimit)

	probe := &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			TCPSocket: &corev1.TCPSocketAction{Port: intstr.FromInt(6000)},
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
			Name:      job.Name,
			Namespace: job.Namespace,
			Labels:    map[string]string{"job": job.Name},
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"job": job.Name},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"job": job.Name},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:            job.Name,
							Image:           job.Spec.Deploy.Image,
							ImagePullPolicy: job.Spec.Deploy.ImagePullPolicy,
							EnvFrom: []corev1.EnvFromSource{
								{
									ConfigMapRef: &corev1.ConfigMapEnvSource{
										LocalObjectReference: corev1.LocalObjectReference{
											Name: job.Name,
										},
									},
								},
							},
							LivenessProbe:  probe,
							ReadinessProbe: probe,
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      job.Name,
									MountPath: "/data/" + job.Name,
								},
							},
							Ports: []corev1.ContainerPort{
								{ContainerPort: 6000, Name: "api", Protocol: "TCP"},
							},
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{corev1.ResourceLimitsMemory: limitMemory, corev1.ResourceLimitsCPU: limitCPU},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: job.Name,
							VolumeSource: corev1.VolumeSource{
								PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: job.Name,
								},
							},
						},
					},
					ImagePullSecrets: job.Spec.Deploy.ImagePullSecrets,
					NodeName:         job.Spec.Deploy.NodeName,
					Affinity:         job.Spec.Deploy.Affinity,
					Tolerations:      job.Spec.Deploy.Tolerations,
				},
			},
		},
	}
	// if job.Spec.Deploy.ImagePullPolicy != nil {

	// }
	if r.Domain != "" && r.Image != "" {
		frontProbe := &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				TCPSocket: &corev1.TCPSocketAction{Port: intstr.FromInt(80)},
			},
			InitialDelaySeconds: 10,
			TimeoutSeconds:      5,
			PeriodSeconds:       30,
			SuccessThreshold:    1,
			FailureThreshold:    5,
		}
		frontContainer := corev1.Container{
			Name:            job.Name + "-front",
			Image:           r.Image,
			ImagePullPolicy: job.Spec.Deploy.ImagePullPolicy,
			LivenessProbe:   frontProbe,
			ReadinessProbe:  frontProbe,
			VolumeMounts: []corev1.VolumeMount{
				{
					Name:      job.Name,
					MountPath: "/data",
				},
			},
			Ports: []corev1.ContainerPort{
				{ContainerPort: 80, Name: "front", Protocol: "TCP"},
			},
		}
		app.Spec.Template.Spec.Containers = append(app.Spec.Template.Spec.Containers, frontContainer)
	}

	if err := ctrl.SetControllerReference(&job, &app, r.Scheme); err != nil {
		return app, err
	}
	return app, nil
}

func (r *JobReconciler) desiredService(job jobsv1beta1.Job) (corev1.Service, error) {
	svc := corev1.Service{
		TypeMeta: metav1.TypeMeta{APIVersion: corev1.SchemeGroupVersion.String(), Kind: "Service"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      job.Name,
			Namespace: job.Namespace,
			Labels:    map[string]string{"job": job.Name},
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{Name: "api", Port: 6000, Protocol: "TCP", TargetPort: intstr.FromInt(6000)},
			},
			Selector: map[string]string{"job": job.Name},
			Type:     corev1.ServiceTypeClusterIP,
		},
	}
	if r.Domain != "" && r.Image != "" {
		svc.Spec.Ports = append(svc.Spec.Ports, corev1.ServicePort{Name: "front", Port: 80, Protocol: "TCP", TargetPort: intstr.FromInt(80)})
	}

	if err := ctrl.SetControllerReference(&job, &svc, r.Scheme); err != nil {
		return svc, err
	}
	return svc, nil
}

func (r *JobReconciler) desiredIngress(job jobsv1beta1.Job) (extensionsv1.Ingress, error) {
	ingr := extensionsv1.Ingress{
		TypeMeta: metav1.TypeMeta{APIVersion: extensionsv1.SchemeGroupVersion.String(), Kind: "Ingress"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      job.Name,
			Namespace: job.Namespace,
			Labels:    map[string]string{"job": job.Name},
		},
		Spec: extensionsv1.IngressSpec{
			Rules: []extensionsv1.IngressRule{
				{
					Host: r.Domain,
					IngressRuleValue: extensionsv1.IngressRuleValue{
						HTTP: &extensionsv1.HTTPIngressRuleValue{
							Paths: []extensionsv1.HTTPIngressPath{
								{
									Path: "/" + job.Name,
									Backend: extensionsv1.IngressBackend{
										ServiceName: job.Name,
										ServicePort: intstr.FromInt(80),
									},
								},
							},
						},
					},
				},
			},
		},
	}

	if err := ctrl.SetControllerReference(&job, &ingr, r.Scheme); err != nil {
		return ingr, err
	}
	return ingr, nil
}
