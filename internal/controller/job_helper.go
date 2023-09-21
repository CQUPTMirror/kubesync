package controller

import (
	"errors"
	"fmt"
	"github.com/CQUPTMirror/kubesync/api/v1beta1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"strconv"
	"strings"
)

const (
	ApiPort   = 6000
	FrontPort = 80
	RsyncPort = 873
)

func (r *JobReconciler) checkRsyncFront(job *v1beta1.Job) (disableFront, disableRsync bool, frontImage, rsyncImage string) {
	frontImage, rsyncImage = r.Config.FrontImage, r.Config.RsyncImage
	if s, err := strconv.ParseBool(job.Spec.Deploy.DisableFront); err == nil {
		disableFront = s
	}
	if job.Spec.Deploy.FrontImage != "" {
		frontImage = job.Spec.Deploy.FrontImage
	}
	if frontImage == "" {
		disableFront = true
	}
	if s, err := strconv.ParseBool(job.Spec.Deploy.DisableRsync); err == nil {
		disableRsync = s
	}
	if job.Spec.Deploy.RsyncImage != "" {
		rsyncImage = job.Spec.Deploy.RsyncImage
	}
	if rsyncImage == "" {
		disableRsync = true
	}
	return
}

func (r *JobReconciler) desiredPersistentVolumeClaim(job *v1beta1.Job) (*corev1.PersistentVolumeClaim, error) {
	pvc := corev1.PersistentVolumeClaim{
		TypeMeta: metav1.TypeMeta{APIVersion: corev1.SchemeGroupVersion.String(), Kind: "PersistentVolumeClaim"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      job.Name,
			Namespace: job.Namespace,
			Labels:    map[string]string{"job": job.Name},
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse(job.Spec.Volume.Size)},
			},
		},
	}
	if job.Spec.Volume.AccessMode != "" {
		pvc.Spec.AccessModes = []corev1.PersistentVolumeAccessMode{job.Spec.Volume.AccessMode}
	} else {
		pvc.Spec.AccessModes = []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce}
	}
	if job.Spec.Volume.StorageClass != nil {
		pvc.Spec.StorageClassName = job.Spec.Volume.StorageClass
	}

	if err := ctrl.SetControllerReference(job, &pvc, r.Scheme); err != nil {
		return &pvc, err
	}
	return &pvc, nil
}

func (r *JobReconciler) desiredDeployment(job *v1beta1.Job, manager string) (*appsv1.Deployment, error) {
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
					Containers: []corev1.Container{},
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
				},
			},
		},
	}

	if job.Status.Status != v1beta1.Disabled {
		env := []corev1.EnvVar{
			{Name: "NAME", Value: job.Name},
			{Name: "PROVIDER", Value: job.Spec.Config.Provider},
			{Name: "UPSTREAM", Value: job.Spec.Config.Upstream},
			{Name: "CONCURRENT", Value: strconv.Itoa(job.Spec.Config.Concurrent)},
			{Name: "INTERVAL", Value: strconv.Itoa(job.Spec.Config.Interval)},
			{Name: "RETRY", Value: strconv.Itoa(job.Spec.Config.Retry)},
			{Name: "TIMEOUT", Value: strconv.Itoa(job.Spec.Config.Timeout)},
			{Name: "COMMAND", Value: job.Spec.Config.Command},
			{Name: "FAIL_ON_MATCH", Value: job.Spec.Config.FailOnMatch},
			{Name: "SIZE_PATTERN", Value: job.Spec.Config.SizePattern},
			{Name: "IPV6", Value: job.Spec.Config.IPv6Only},
			{Name: "IPV4", Value: job.Spec.Config.IPv4Only},
			{Name: "EXCLUDE_FILE", Value: job.Spec.Config.ExcludeFile},
			{Name: "RSYNC_OPTIONS", Value: job.Spec.Config.RsyncOptions},
			{Name: "STAGE1_PROFILE", Value: job.Spec.Config.Stage1Profile},
			{Name: "EXEC_ON_SUCCESS", Value: job.Spec.Config.ExecOnSuccess},
			{Name: "EXEC_ON_FAILURE", Value: job.Spec.Config.ExecOnFailure},
			{Name: "API", Value: fmt.Sprintf("http://%s:3000", manager)},
			{Name: "ADDR", Value: fmt.Sprintf(":%d", ApiPort)},
		}
		if job.Spec.Config.AdditionEnvs != "" {
			for _, item := range strings.Split(job.Spec.Config.AdditionEnvs, ";") {
				splits := strings.Split(item, "=")
				if len(splits) == 2 {
					env = append(env, corev1.EnvVar{Name: splits[0], Value: splits[1]})
				}
			}
		}
		if job.Spec.Config.Provider == "" || job.Spec.Config.Upstream == "" {
			return nil, errors.New("provider or upstream not set")
		}
		if job.Spec.Config.Debug != "" {
			env = append(env, corev1.EnvVar{Name: "DEBUG", Value: "true"})
		}
		probe := &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				TCPSocket: &corev1.TCPSocketAction{Port: intstr.FromInt(ApiPort)},
			},
			InitialDelaySeconds: 10,
			TimeoutSeconds:      5,
			PeriodSeconds:       30,
			SuccessThreshold:    1,
			FailureThreshold:    5,
		}
		container := corev1.Container{
			Name:           job.Name,
			Image:          job.Spec.Deploy.Image,
			Env:            env,
			LivenessProbe:  probe,
			ReadinessProbe: probe,
			VolumeMounts: []corev1.VolumeMount{
				{
					Name:      job.Name,
					MountPath: "/data/" + job.Name,
				},
			},
			Ports: []corev1.ContainerPort{
				{ContainerPort: ApiPort, Name: "api", Protocol: "TCP"},
			},
		}
		if job.Spec.Deploy.ImagePullPolicy != "" {
			container.ImagePullPolicy = job.Spec.Deploy.ImagePullPolicy
		}
		if job.Spec.Deploy.MemoryLimit != "" || job.Spec.Deploy.CPULimit != "" {
			container.Resources = corev1.ResourceRequirements{Limits: corev1.ResourceList{}}
			if job.Spec.Deploy.MemoryLimit != "" {
				container.Resources.Limits[corev1.ResourceLimitsMemory] = resource.MustParse(job.Spec.Deploy.MemoryLimit)
			}
			if job.Spec.Deploy.CPULimit != "" {
				container.Resources.Limits[corev1.ResourceLimitsCPU] = resource.MustParse(job.Spec.Deploy.CPULimit)
			}
		}
		app.Spec.Template.Spec.Containers = append(app.Spec.Template.Spec.Containers, container)
	}

	if job.Spec.Deploy.ImagePullSecrets != nil {
		app.Spec.Template.Spec.ImagePullSecrets = job.Spec.Deploy.ImagePullSecrets
	}
	if job.Spec.Deploy.NodeName != "" {
		app.Spec.Template.Spec.NodeName = job.Spec.Deploy.NodeName
	}
	if job.Spec.Deploy.Affinity != nil {
		app.Spec.Template.Spec.Affinity = job.Spec.Deploy.Affinity
	}
	if job.Spec.Deploy.Tolerations != nil {
		app.Spec.Template.Spec.Tolerations = job.Spec.Deploy.Tolerations
	}
	disableFront, disableRsync, frontImage, rsyncImage := r.checkRsyncFront(job)
	if !disableFront {
		frontProbe := &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				TCPSocket: &corev1.TCPSocketAction{Port: intstr.FromInt(FrontPort)},
			},
			InitialDelaySeconds: 10,
			TimeoutSeconds:      5,
			PeriodSeconds:       30,
			SuccessThreshold:    1,
			FailureThreshold:    5,
		}
		frontContainer := corev1.Container{
			Name:            job.Name + "-front",
			Image:           frontImage,
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
				{ContainerPort: FrontPort, Name: "front", Protocol: "TCP"},
			},
		}
		app.Spec.Template.Spec.Containers = append(app.Spec.Template.Spec.Containers, frontContainer)
	}
	if !disableRsync {
		rsyncProbe := &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				TCPSocket: &corev1.TCPSocketAction{Port: intstr.FromInt(RsyncPort)},
			},
			InitialDelaySeconds: 10,
			TimeoutSeconds:      5,
			PeriodSeconds:       30,
			SuccessThreshold:    1,
			FailureThreshold:    5,
		}
		rsyncContainer := corev1.Container{
			Name:            job.Name + "-rsync",
			Image:           rsyncImage,
			ImagePullPolicy: job.Spec.Deploy.ImagePullPolicy,
			LivenessProbe:   rsyncProbe,
			ReadinessProbe:  rsyncProbe,
			VolumeMounts: []corev1.VolumeMount{
				{
					Name:      job.Name,
					MountPath: "/data",
				},
			},
			Ports: []corev1.ContainerPort{
				{ContainerPort: RsyncPort, Name: "rsync", Protocol: "TCP"},
			},
		}
		app.Spec.Template.Spec.Containers = append(app.Spec.Template.Spec.Containers, rsyncContainer)
	}

	if len(app.Spec.Template.Spec.Containers) == 0 {
		return nil, nil
	}

	if err := ctrl.SetControllerReference(job, &app, r.Scheme); err != nil {
		return &app, err
	}
	return &app, nil
}

func (r *JobReconciler) desiredService(job *v1beta1.Job) (*corev1.Service, error) {
	svc := corev1.Service{
		TypeMeta: metav1.TypeMeta{APIVersion: corev1.SchemeGroupVersion.String(), Kind: "Service"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      job.Name,
			Namespace: job.Namespace,
			Labels:    map[string]string{"job": job.Name},
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{Name: "api", Port: ApiPort, Protocol: "TCP", TargetPort: intstr.FromString("api")},
			},
			Selector: map[string]string{"job": job.Name},
			Type:     corev1.ServiceTypeClusterIP,
		},
	}
	disableFront, disableRsync, _, _ := r.checkRsyncFront(job)
	if !disableFront {
		svc.Spec.Ports = append(svc.Spec.Ports, corev1.ServicePort{Name: "front", Port: FrontPort, Protocol: "TCP", TargetPort: intstr.FromString("front")})
	}
	if !disableRsync {
		svc.Spec.Ports = append(svc.Spec.Ports, corev1.ServicePort{Name: "rsync", Port: RsyncPort, Protocol: "TCP", TargetPort: intstr.FromString("rsync")})
	}

	if err := ctrl.SetControllerReference(job, &svc, r.Scheme); err != nil {
		return &svc, err
	}
	return &svc, nil
}

func (r *JobReconciler) desiredIngress(job *v1beta1.Job) (*v1.Ingress, error) {
	annotations := make(map[string]string)
	for k, v := range r.Config.FrontAnn {
		annotations[k] = v
	}
	for k, v := range job.Spec.Ingress.Annotations {
		annotations[k] = v
	}

	pathType := v1.PathTypePrefix

	ig := v1.Ingress{
		TypeMeta: metav1.TypeMeta{APIVersion: v1.SchemeGroupVersion.String(), Kind: "Ingress"},
		ObjectMeta: metav1.ObjectMeta{
			Name:        job.Name,
			Namespace:   job.Namespace,
			Labels:      map[string]string{"job": job.Name},
			Annotations: annotations,
		},
		Spec: v1.IngressSpec{
			Rules: []v1.IngressRule{
				{
					IngressRuleValue: v1.IngressRuleValue{
						HTTP: &v1.HTTPIngressRuleValue{
							Paths: []v1.HTTPIngressPath{
								{
									Path:     "/" + job.Name,
									PathType: &pathType,
									Backend: v1.IngressBackend{
										Service: &v1.IngressServiceBackend{
											Name: job.Name,
											Port: v1.ServiceBackendPort{Name: "front"},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	if r.Config.FrontClass != "" || job.Spec.Ingress.IngressClass != "" {
		ig.Spec.IngressClassName = &r.Config.FrontClass
		if job.Spec.Ingress.IngressClass != "" {
			ig.Spec.IngressClassName = &job.Spec.Ingress.IngressClass
		}
	}

	if r.Config.FrontTLS != "" || job.Spec.Ingress.TLSSecret != "" {
		secretName := r.Config.FrontTLS
		if job.Spec.Ingress.TLSSecret != "" {
			secretName = job.Spec.Ingress.TLSSecret
		}
		ig.Spec.TLS = []v1.IngressTLS{{SecretName: secretName}}
	}

	if r.Config.FrontHost != "" || job.Spec.Ingress.Host != "" {
		ig.Spec.Rules[0].Host = r.Config.FrontHost
		if job.Spec.Ingress.Host != "" {
			ig.Spec.Rules[0].Host = job.Spec.Ingress.Host
		}
	}

	if job.Spec.Ingress.Path != "" {
		ig.Spec.Rules[0].IngressRuleValue.HTTP.Paths[0].Path = job.Spec.Ingress.Path
	}

	if err := ctrl.SetControllerReference(job, &ig, r.Scheme); err != nil {
		return &ig, err
	}
	return &ig, nil
}
