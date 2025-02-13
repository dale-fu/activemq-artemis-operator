package v2alpha5activemqartemis

import (
	"context"
	"fmt"

	brokerv2alpha5 "github.com/artemiscloud/activemq-artemis-operator/pkg/apis/broker/v2alpha5"
	nsoptions "github.com/artemiscloud/activemq-artemis-operator/pkg/resources/namespaces"
	"github.com/artemiscloud/activemq-artemis-operator/pkg/utils/common"
	"github.com/artemiscloud/activemq-artemis-operator/pkg/utils/lsrcrs"

	"github.com/artemiscloud/activemq-artemis-operator/pkg/utils/selectors"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var log = logf.Log.WithName("controller_v2alpha5activemqartemis")

var namespacedNameToFSM = make(map[types.NamespacedName]*ActiveMQArtemisFSM)

type ActiveMQArtemisConfigHandler interface {
	IsApplicableFor(brokerNamespacedName types.NamespacedName) bool
	Config(initContainers []corev1.Container, outputDirRoot string, yacfgProfileVersion string, yacfgProfileName string) (value []string)
}

var namespaceToConfigHandler = make(map[types.NamespacedName]ActiveMQArtemisConfigHandler)

func GetBrokerConfigHandler(brokerNamespacedName types.NamespacedName) (handler ActiveMQArtemisConfigHandler) {
	for _, handler := range namespaceToConfigHandler {
		if handler.IsApplicableFor(brokerNamespacedName) {
			return handler
		}
	}
	return nil
}

func UpdatePodForSecurity(securityHandlerNamespacedName types.NamespacedName, handler ActiveMQArtemisConfigHandler) error {
	success := true
	for nsn, fsm := range namespacedNameToFSM {
		if handler.IsApplicableFor(nsn) {
			fsm.SetPodInvalid(true)
			log.Info("Need update fsm for security", "fsm", nsn)
			if err, _ := fsm.Update(); err != nil {
				success = false
				log.Error(err, "error in updating security", "cr", fsm.namespacedName)
			}
		}
	}
	if success {
		return nil
	}
	err := fmt.Errorf("error in update security, please see log for details")
	return err
}

func RemoveBrokerConfigHandler(namespacedName types.NamespacedName) {
	log.Info("Removing config handler", "name", namespacedName)
	oldHandler, ok := namespaceToConfigHandler[namespacedName]
	if ok {
		delete(namespaceToConfigHandler, namespacedName)
		log.Info("Handler removed, updating fsm if exists")
		UpdatePodForSecurity(namespacedName, oldHandler)
	}
}

func AddBrokerConfigHandler(namespacedName types.NamespacedName, handler ActiveMQArtemisConfigHandler, toReconcile bool) error {
	if _, ok := namespaceToConfigHandler[namespacedName]; ok {
		log.V(1).Info("There is an old config handler, it'll be replaced")
	}
	namespaceToConfigHandler[namespacedName] = handler
	log.V(1).Info("A new config handler has been added", "handler", handler)
	if toReconcile {
		log.V(1).Info("Updating broker security")
		return UpdatePodForSecurity(namespacedName, handler)
	}
	return nil
}

/**
* USER ACTION REQUIRED: This is a scaffold file intended for the user to modify with their own Controller
* business logic.  Delete these comments after modifying this file.*
 */

// Add creates a new ActiveMQArtemis Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileActiveMQArtemis{client: mgr.GetClient(), scheme: mgr.GetScheme(), result: reconcile.Result{Requeue: false}}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("v2alpha5activemqartemis-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource ActiveMQArtemis
	err = c.Watch(&source.Kind{Type: &brokerv2alpha5.ActiveMQArtemis{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// TODO(user): Modify this to be the types you create that are owned by the primary resource
	// Watch for changes to secondary resource Pods and requeue the owner ActiveMQArtemis
	err = c.Watch(&source.Kind{Type: &appsv1.StatefulSet{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &brokerv2alpha5.ActiveMQArtemis{},
	})
	if err != nil {
		return err
	}

	// Watch for changes to secondary resource Pods and requeue the owner ActiveMQArtemis
	err = c.Watch(&source.Kind{Type: &corev1.Pod{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &brokerv2alpha5.ActiveMQArtemis{},
	})
	if err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileActiveMQArtemis{}

// ReconcileActiveMQArtemis reconciles a ActiveMQArtemis object
type ReconcileActiveMQArtemis struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client client.Client
	scheme *runtime.Scheme
	result reconcile.Result
}

// Reconcile reads that state of the cluster for a ActiveMQArtemis object and makes changes based on the state read
// and what is in the ActiveMQArtemis.Spec
// TODO(user): Modify this Reconcile function to implement your Controller logic.  This example creates
// a Pod as an example
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileActiveMQArtemis) Reconcile(request reconcile.Request) (reconcile.Result, error) {

	// Log where we are and what we're doing
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling ActiveMQArtemis")

	if !nsoptions.Match(request.Namespace) {
		reqLogger.Info("Request not in watch list, ignore", "request", request)
		return reconcile.Result{}, nil
	}

	var err error = nil
	var namespacedNameFSM *ActiveMQArtemisFSM = nil
	var amqbfsm *ActiveMQArtemisFSM = nil

	customResource := &brokerv2alpha5.ActiveMQArtemis{}
	namespacedName := types.NamespacedName{
		Name:      request.Name,
		Namespace: request.Namespace,
	}

	// Fetch the ActiveMQArtemis instance
	// When first creating this will have err == nil
	// When deleting after creation this will have err NotFound
	// When deleting before creation reconcile won't be called
	if err = r.client.Get(context.TODO(), request.NamespacedName, customResource); err != nil {
		if errors.IsNotFound(err) {
			reqLogger.Info("ActiveMQArtemis Controller Reconcile encountered a IsNotFound, checking to see if we should delete namespacedName tracking for request NamespacedName " + request.NamespacedName.String())

			// See if we have been tracking this NamespacedName
			if namespacedNameFSM = namespacedNameToFSM[namespacedName]; namespacedNameFSM != nil {
				reqLogger.Info("Removing namespacedName tracking for " + namespacedName.String())
				// If so we should no longer track it
				amqbfsm = namespacedNameToFSM[namespacedName]
				//remove the fsm secret
				lsrcrs.DeleteLastSuccessfulReconciledCR(request.NamespacedName, "broker", amqbfsm.namers.LabelBuilder.Labels(), r.client)
				amqbfsm.Exit()
				delete(namespacedNameToFSM, namespacedName)
				amqbfsm = nil
			}

			// Setting err to nil to prevent requeue
			err = nil
		} else {
			reqLogger.Error(err, "ActiveMQArtemis Controller Reconcile errored thats not IsNotFound, requeuing request", "Request Namespace", request.Namespace, "Request Name", request.Name)
			// Leaving err as !nil causes requeue
		}

		// Add error detail for use later
		return r.result, err
	}

	// Do lookup to see if we have a fsm for the incoming name in the incoming namespace
	// if not, create it
	// for the given fsm, do an update
	// - update first level sets? what if the operator has gone away and come back? stateless?
	if namespacedNameFSM = namespacedNameToFSM[namespacedName]; namespacedNameFSM == nil {
		log.Info("Didn't find fsm for the CR, try to search history", "requested", namespacedName)
		//try to retrieve last successful reconciled CR
		lsrcr := lsrcrs.RetrieveLastSuccessfulReconciledCR(namespacedName, "broker", r.client, GetDefaultLabels(customResource))
		if lsrcr != nil {
			log.Info("There is a LastSuccessfulReconciledCR")
			//restoring fsm
			var fsmData ActiveMQArtemisFSMData
			var fsm *ActiveMQArtemisFSM
			if merr := common.FromJson(&lsrcr.Data, &fsmData); merr != nil {
				log.Error(merr, "failed to unmarshal fsm, create a new one")
				fsm = MakeActiveMQArtemisFSM(customResource, namespacedName, r)
			} else {
				log.Info("recreate fsm from data")
				storedCR := brokerv2alpha5.ActiveMQArtemis{}
				merr := common.FromJson(&lsrcr.CR, &storedCR)
				if merr != nil {
					log.Error(merr, "failed to unmarshal cr, using existing one")
					fsm = MakeActiveMQArtemisFSMFromData(&fsmData, customResource, namespacedName, r)
				} else {
					log.Info("Restoring fsm")
					fsm = MakeActiveMQArtemisFSMFromData(&fsmData, &storedCR, namespacedName, r)
				}
			}
			namespacedNameToFSM[namespacedName] = fsm
			if lsrcr.Checksum == customResource.ResourceVersion {
				//this is an operator restart. Don't do reconcile
				log.Info("Detected possible operator restart with no broker CR changes", "res", customResource.ResourceVersion)
				return r.result, nil
			}
			log.Info("A new version of CR comes in", "old", lsrcr.Checksum, "new", customResource.ResourceVersion)
		}
	}

	if namespacedNameFSM = namespacedNameToFSM[namespacedName]; namespacedNameFSM == nil {

		amqbfsm = MakeActiveMQArtemisFSM(customResource, namespacedName, r)
		namespacedNameToFSM[namespacedName] = amqbfsm

		// Enter the first state; atm CreatingK8sResourcesState
		amqbfsm.Enter(CreatingK8sResourcesID)
	} else {
		amqbfsm = namespacedNameFSM
		//remember current customeResource so that we can compare for update
		amqbfsm.UpdateCustomResource(customResource)

		err, _ = amqbfsm.Update()
	}

	//persist the CR
	if err == nil {
		fsmData := amqbfsm.GetFSMData()
		fsmstr, merr := common.ToJson(&fsmData)
		if merr != nil {
			log.Error(merr, "failed to marshal fsm")
		}
		crstr, merr := common.ToJson(customResource)
		if merr != nil {
			log.Error(merr, "failed to marshal cr")
		}
		lsrcrs.StoreLastSuccessfulReconciledCR(customResource, customResource.Name,
			customResource.Namespace, "broker", crstr, fsmstr, customResource.ResourceVersion,
			amqbfsm.namers.LabelBuilder.Labels(), r.client, r.scheme)
	}
	// Single exit, return the result and error condition
	return r.result, err
}

func GetDefaultLabels(cr *brokerv2alpha5.ActiveMQArtemis) map[string]string {
	defaultLabelData := selectors.LabelerData{}
	defaultLabelData.Base(cr.Name).Suffix("app").Generate()
	return defaultLabelData.Labels()
}

type StatefulSetInfo struct {
	NamespacedName types.NamespacedName
	Labels         map[string]string
}

//get the statefulset names
func GetDeployedStatefuleSetNames(targetCrNames []types.NamespacedName) []StatefulSetInfo {

	var result []StatefulSetInfo = nil

	if len(targetCrNames) == 0 {
		for _, fsm := range namespacedNameToFSM {
			info := StatefulSetInfo{
				NamespacedName: fsm.GetStatefulSetNamespacedName(),
				Labels:         fsm.namers.LabelBuilder.Labels(),
			}
			result = append(result, info)
		}
		return result
	}

	for _, target := range targetCrNames {
		log.Info("Trying to get target fsm", "target", target)
		if fsm := namespacedNameToFSM[target]; fsm != nil {
			log.Info("got fsm", "fsm", fsm, "ss namer", fsm.namers.SsNameBuilder.Name())
			info := StatefulSetInfo{
				NamespacedName: fsm.GetStatefulSetNamespacedName(),
				Labels:         fsm.namers.LabelBuilder.Labels(),
			}
			result = append(result, info)
		}
	}
	return result
}
