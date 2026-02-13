package controller

import (
	"context"
	"fmt"

	v1 "k8s.io/api/autoscaling/v1"
	batchv1 "k8s.io/api/batch/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	vpav1 "k8s.io/autoscaler/vertical-pod-autoscaler/pkg/apis/autoscaling.k8s.io/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

type JobReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

func (r *JobReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	l := log.FromContext(ctx)

	var job batchv1.Job
	err := r.Get(ctx, req.NamespacedName, &job)
	if err != nil {
		if errors.IsNotFound(err) {
			// Job was deleted — VPA will be automatically garbage collected by Kubernetes
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	vpaName := fmt.Sprintf("%s-vpa", job.Name)

	// Check if VPA already exists
	var existingVPA vpav1.VerticalPodAutoscaler
	err = r.Get(ctx, client.ObjectKey{Name: vpaName, Namespace: job.Namespace}, &existingVPA)
	if err == nil {
		// VPA exists
		return ctrl.Result{}, nil
	} else if !errors.IsNotFound(err) {
		return ctrl.Result{}, err
	}

	// Create new VPA
	vpa := &vpav1.VerticalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Name:      vpaName,
			Namespace: job.Namespace,
		},
		Spec: vpav1.VerticalPodAutoscalerSpec{
			TargetRef: &v1.CrossVersionObjectReference{
				Kind:       "Job",
				Name:       job.Name,
				APIVersion: "batch/v1",
			},
			UpdatePolicy: &vpav1.PodUpdatePolicy{
				UpdateMode: func() *vpav1.UpdateMode {
					mode := vpav1.UpdateModeOff
					return &mode
				}(),
			},
		},
	}

	// Set the Job as the owner of the VPA for automatic garbage collection
	if err := controllerutil.SetControllerReference(&job, vpa, r.Scheme); err != nil {
		l.Error(err, "Failed to set controller reference")
		return ctrl.Result{}, err
	}

	if err := r.Create(ctx, vpa); err != nil {
		l.Error(err, "Failed to create VPA")
		return ctrl.Result{}, err
	}

	l.Info("Created VPA", "VPA", vpaName)
	return ctrl.Result{}, nil
}

func (r *JobReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&batchv1.Job{}).
		Complete(r)
}
