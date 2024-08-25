package controller

import "github.com/CQUPTMirror/kubesync/api/v1beta1"

func getCommonLabels(job *v1beta1.Job) map[string]string {
	labels := map[string]string{
		"kubernetes.io/app":        job.Name,
		"kubernetes.io/component":  "mirror",
		"kubernetes.io/managed-by": "kubesync",
	}

	return labels
}

func serviceName(jobName string) string {
	// we have to add a prefix to avoid conflict when enableServiceLinks is true and the service/job name is "kubernetes". because enableServiceLinks will
	// add an environment variable "KUBERNETES_SERVICE_HOST" to the pod, which the environment points to kubernetes api server by default.
	return "mirror-" + jobName
}
