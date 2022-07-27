package recommendation

import (
	"context"
	"fmt"
	recommender "github.com/gocrane/crane/pkg/recommendation"
	"github.com/gocrane/crane/pkg/recommendation/recommender/apis"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/scale"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	analysisv1alph1 "github.com/gocrane/api/analysis/v1alpha1"

	"github.com/gocrane/crane/pkg/providers"
)

// Controller is responsible for reconcile Recommendation
type Controller struct {
	client.Client
	Config         apis.RecommenderConfiguration
	Scheme         *runtime.Scheme
	Recorder       record.EventRecorder
	RestMapper     meta.RESTMapper
	ScaleClient    scale.ScalesGetter
	RecommenderMgr recommender.RecommenderManager
	Provider       providers.Interface
}

func (c *Controller) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	klog.V(4).Infof("Got Recommendation %s", req.NamespacedName)

	recommendation := &analysisv1alph1.Recommendation{}
	err := c.Client.Get(ctx, req.NamespacedName, recommendation)
	if err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if recommendation.DeletionTimestamp != nil {
		return ctrl.Result{}, nil
	}

	// defaulting for TargetRef.Namespace
	if recommendation.Spec.TargetRef.Namespace == "" {
		recommendation.Spec.TargetRef.Namespace = recommendation.Namespace
	}

	newStatus := recommendation.Status.DeepCopy()

	updated, err := c.UpdateRecommendation(ctx, recommendation)
	if err != nil {
		c.Recorder.Event(recommendation, v1.EventTypeWarning, "FailedUpdateRecommendationValue", err.Error())
		msg := fmt.Sprintf("Failed to update recommendation value, Recommendation %s: %v", klog.KObj(recommendation), err)
		klog.Errorf(msg)
		setReadyCondition(newStatus, metav1.ConditionFalse, "FailedUpdateRecommendationValue", msg)
		c.UpdateStatus(ctx, recommendation, newStatus)
		return ctrl.Result{}, err
	}

	if updated {
		c.Recorder.Event(recommendation, v1.EventTypeNormal, "UpdatedRecommendationValue", "")

		setReadyCondition(newStatus, metav1.ConditionTrue, "RecommendationReady", "Recommendation is ready")
		c.UpdateStatus(ctx, recommendation, newStatus)
	}

	return ctrl.Result{}, nil
}

func (c *Controller) UpdateStatus(ctx context.Context, recommendation *analysisv1alph1.Recommendation, newStatus *analysisv1alph1.RecommendationStatus) {
	if !equality.Semantic.DeepEqual(&recommendation.Status, newStatus) {
		recommendation.Status = *newStatus
		timeNow := metav1.Now()
		recommendation.Status.LastUpdateTime = &timeNow

		err := c.Update(ctx, recommendation)
		if err != nil {
			c.Recorder.Event(recommendation, v1.EventTypeWarning, "FailedUpdateStatus", err.Error())
			klog.Errorf("Failed to update status, Recommendation %s error %v", klog.KObj(recommendation), err)
			return
		}

		klog.Infof("Update Recommendation status successful, Recommendation %s", klog.KObj(recommendation))
	}
}

func (c *Controller) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&analysisv1alph1.Recommendation{}).
		Complete(c)
}

func setReadyCondition(status *analysisv1alph1.RecommendationStatus, conditionStatus metav1.ConditionStatus, reason string, message string) {
	for i := range status.Conditions {
		if status.Conditions[i].Type == "Ready" {
			status.Conditions[i].Status = conditionStatus
			status.Conditions[i].Reason = reason
			status.Conditions[i].Message = message
			status.Conditions[i].LastTransitionTime = metav1.Now()
			return
		}
	}
	status.Conditions = append(status.Conditions, metav1.Condition{
		Type:               "Ready",
		Status:             conditionStatus,
		Reason:             reason,
		Message:            message,
		LastTransitionTime: metav1.Now(),
	})
}
