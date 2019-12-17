package nodejsboot

import (
	"context"
	"github.com/go-logr/logr"
	appv1 "github.com/logancloud/logan-app-operator/pkg/apis/app/v1"
	"github.com/logancloud/logan-app-operator/pkg/logan"
	"github.com/logancloud/logan-app-operator/pkg/logan/config"
	loganMetrics "github.com/logancloud/logan-app-operator/pkg/logan/metrics"
	"github.com/logancloud/logan-app-operator/pkg/logan/operator"
	"github.com/logancloud/logan-app-operator/pkg/logan/util"
	"github.com/logancloud/logan-app-operator/pkg/logan/util/keys"
	appsv1 "k8s.io/api/apps/v1"
	autoscaling "k8s.io/api/autoscaling/v2beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/source"
	"time"
)

var log = logf.Log.WithName("logan_controller_nodejsboot")
var bootType = "NodeJSBoot"

// Add creates a new NodeJSBoot Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileNodeJSBoot{
		client:   util.NewClient(mgr.GetClient()),
		scheme:   mgr.GetScheme(),
		recorder: mgr.GetEventRecorderFor("nodejsboot-controller"),
	}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("nodejsboot-controller", mgr,
		controller.Options{Reconciler: r, MaxConcurrentReconciles: logan.MaxConcurrentReconciles})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource NodeJSBoot
	err = c.Watch(&source.Kind{Type: &appv1.NodeJSBoot{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// Modify this to be the types you create(Deployment and Service) that are owned by the primary resource
	err = c.Watch(&source.Kind{Type: &appsv1.Deployment{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &appv1.NodeJSBoot{},
	})
	if err != nil {
		return err
	}

	err = c.Watch(&source.Kind{Type: &appsv1.StatefulSet{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &appv1.NodeJSBoot{},
	})
	if err != nil {
		return err
	}

	err = c.Watch(&source.Kind{Type: &corev1.Service{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &appv1.NodeJSBoot{},
	})
	if err != nil {
		return err
	}

	err = c.Watch(&source.Kind{Type: &autoscaling.HorizontalPodAutoscaler{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &appv1.NodeJSBoot{},
	})
	if err != nil {
		return err
	}

	return nil
}

// blank assignment to verify that ReconcileNodeJSBoot implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileNodeJSBoot{}

// ReconcileNodeJSBoot reconciles a NodeJSBoot object
type ReconcileNodeJSBoot struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client   util.K8SClient
	scheme   *runtime.Scheme
	recorder record.EventRecorder
}

// Reconcile reads that state of the cluster for a NodeJSBoot object and makes changes based on the state read
// and what is in the NodeJSBoot.Spec
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileNodeJSBoot) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	logger := log.WithValues("nodejsboot", request)

	if operator.Ignore(request.Namespace) {
		return reconcile.Result{}, nil
	}

	logger.Info("Reconciling NodeJSBoot")
	// Update metrics after processing each Reconcile
	reconcileStartTS := time.Now()
	defer func() {
		loganMetrics.UpdateReconcileTime(bootType, time.Now().Sub(reconcileStartTS))
	}()

	var bootHandler *operator.BootHandler

	// Fetch the Boot instance
	nodejsBoot := &appv1.NodeJSBoot{}
	err := r.client.Get(context.TODO(), request.NamespacedName, nodejsBoot)
	if err != nil {
		loganMetrics.UpdateMainStageErrors(bootType,
			loganMetrics.RECONCILE_GET_BOOT_STAGE,
			request.Name)
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			logger.Info("Boot resource not found. Ignoring since object must be deleted")
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		logger.Error(err, "Failed to get Boot")
		return reconcile.Result{}, err
	}

	bootHandler = InitHandler(nodejsBoot, r.scheme, r.client, logger, r.recorder)

	//if !logan.MutationDefaulter {
	changed := bootHandler.DefaultValue()

	//Update the Boot's default Value
	if changed {
		logger.Info("Updating Boot with Defaulters")
		err = r.client.Update(context.TODO(), nodejsBoot)
		if err != nil {
			msg := "Failed to update Boot with Defaulters"
			logger.Info(msg, "boot", nodejsBoot)
			loganMetrics.UpdateMainStageErrors(bootType,
				loganMetrics.RECONCILE_UPDATE_BOOT_DEFAULTERS_STAGE,
				nodejsBoot.Name)
			bootHandler.RecordEvent(keys.FailedUpdateBootDefaulters, msg, err)
			return reconcile.Result{Requeue: true}, nil
		}
		bootHandler.RecordEvent(keys.UpdatedBootDefaulters, "Updated Boot with Defaulters", nil)
		return reconcile.Result{Requeue: true}, nil
	}
	//}

	// 1. Check the existence of components, if not exist, create new one.
	result, requeue, err := bootHandler.ReconcileCreate()
	if requeue {
		return result, err
	}

	// 2. Handle the update logic of components
	result, requeue, err = bootHandler.ReconcileUpdate()
	if requeue {
		return result, err
	}

	// 3. Handle the update logic of hpa
	result, err = bootHandler.ReconcileHpa()
	if result.Requeue || err != nil {
		return result, err
	}

	// 4. Handle the update logic of status
	result, requeue, updated, err := bootHandler.ReconcileUpdateStatus()
	if updated {
		logger.Info("Updating Boot Status", "Status", nodejsBoot.Status)
		err := r.client.Status().Update(context.TODO(), nodejsBoot)
		if err != nil {
			msg := "Failed to update Boot Status"
			logger.Info(msg, "err", err.Error())
			loganMetrics.UpdateReconcileErrors(bootType,
				loganMetrics.RECONCILE_UPDATE_BOOT_STATUS_STAGE,
				loganMetrics.RECONCILE_UPDATE_BOOT_STATUS_SUBSTAGE,
				nodejsBoot.Name)
			bootHandler.RecordEvent(keys.FailedUpdateBootStatus, msg, err)
			return reconcile.Result{Requeue: true}, err
		}

		bootHandler.RecordEvent(keys.UpdatedBootStatus, "Updated Boot Status", nil)
		return reconcile.Result{Requeue: true}, nil
	}
	if requeue || err != nil {
		return result, err
	}

	// 5. Handle the update logic of boot meta
	result, requeue, updated, err = bootHandler.ReconcileUpdateBootMeta()

	if updated {
		logger.Info("Updating Boot Meta", "new", nodejsBoot.Annotations)
		err := r.client.Update(context.TODO(), nodejsBoot)
		if err != nil {
			// Other place will modify the status? So it will sometimes occur.
			msg := "Failed to update Boot Meta"
			logger.Info(msg, "err", err.Error())
			loganMetrics.UpdateReconcileErrors(bootType,
				loganMetrics.RECONCILE_UPDATE_BOOT_META_STAGE,
				loganMetrics.RECONCILE_UPDATE_BOOT_META_SUBSTAGE,
				nodejsBoot.Name)

			bootHandler.RecordEvent(keys.FailedUpdateBootMeta, msg, err)
			return reconcile.Result{Requeue: true}, err
		}
		bootHandler.RecordEvent(keys.UpdatedBootMeta, "Updated Boot Meta", nil)
	}
	if requeue {
		return result, err
	}

	return reconcile.Result{}, nil
}

// InitHandler will create the Handler for handling logic of Boot
func InitHandler(nodejsBoot *appv1.NodeJSBoot, scheme *runtime.Scheme,
	client util.K8SClient, logger logr.Logger, recorder record.EventRecorder) (handler *operator.BootHandler) {
	boot := nodejsBoot.DeepCopyBoot()

	bootCfg := config.NodeJSConfig
	profileConfig, err := operator.GetProfileBootConfig(boot, logger)
	if err != nil {
		logger.Info(err.Error())
	} else if profileConfig != nil {
		bootCfg = profileConfig
	}

	return &operator.BootHandler{
		OperatorBoot:   nodejsBoot,
		OperatorSpec:   &nodejsBoot.Spec,
		OperatorMeta:   &nodejsBoot.ObjectMeta,
		OperatorStatus: &nodejsBoot.Status,

		Boot:     boot,
		Config:   bootCfg,
		Scheme:   scheme,
		Client:   client,
		Logger:   logger,
		Recorder: recorder,
	}
}
