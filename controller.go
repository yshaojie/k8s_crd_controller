package main

import (
	"fmt"
	"github.com/golang/glog"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	appslisters "k8s.io/client-go/listers/apps/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	crdv1 "k8s_crd_controller/pkg/apis/crd/v1"
	clientset "k8s_crd_controller/pkg/generated/clientset/versioned"
	studentScheme "k8s_crd_controller/pkg/generated/clientset/versioned/scheme"
	informers "k8s_crd_controller/pkg/generated/informers/externalversions/crd/v1"
	listers "k8s_crd_controller/pkg/generated/listers/crd/v1"
	"time"
)

const controllerAgentName = "student-controller"

const (
	// SuccessSynced is used as part of the Event 'reason' when a Foo is synced
	SuccessSynced = "Synced"
	// ErrResourceExists is used as part of the Event 'reason' when a Foo fails
	// to sync due to a Deployment of the same name already existing.
	ErrResourceExists = "ErrResourceExists"

	// MessageResourceExists is the message used for Events when a resource
	// fails to sync due to a Deployment already existing
	MessageResourceExists = "Resource %q already exists and is not managed by Foo"
	// MessageResourceSynced is the message used for an Event fired when a Foo
	// is synced successfully
	MessageResourceSynced = "Foo synced successfully"
)

// Controller is the controller implementation for Foo resources
type Controller struct {
	// kubeclientset is a standard kubernetes clientset
	kubeclientset kubernetes.Interface
	// sampleclientset is a clientset for our own API group
	sampleclientset clientset.Interface

	deploymentsLister appslisters.DeploymentLister
	deploymentsSynced cache.InformerSynced
	studentLister     listers.StudentLister
	studentSynced     cache.InformerSynced

	// workqueue is a rate limited work queue. This is used to queue work to be
	// processed instead of performing it as soon as a change happens. This
	// means we can ensure we only process a fixed amount of resources at a
	// time, and makes it easy to ensure we are never processing the same item
	// simultaneously in two different workers.
	workqueue workqueue.RateLimitingInterface
	// recorder is an event recorder for recording Event resources to the
	// Kubernetes API.
	recorder record.EventRecorder
}

func NewController(
	kubeclientset kubernetes.Interface,
	mydemoslientset clientset.Interface,
	studentInformer informers.StudentInformer) *Controller {

	// Create event broadcaster
	// Add sample-controller types to the default Kubernetes Scheme so Events can be
	// logged for sample-controller types.
	utilruntime.Must(studentScheme.AddToScheme(scheme.Scheme))
	glog.V(4).Info("Creating event broadcaster")
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(glog.Infof)
	eventBroadcaster.StartRecordingToSink(&typedcorev1.EventSinkImpl{Interface: kubeclientset.CoreV1().Events("")})
	recorder := eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: controllerAgentName})

	//使用client和前面创建的Informer，初始化了自定义控制器
	controller := &Controller{
		kubeclientset:   kubeclientset,
		sampleclientset: mydemoslientset,
		studentLister:   studentInformer.Lister(),
		studentSynced:   studentInformer.Informer().HasSynced,
		workqueue:       workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "MyStudent"), //WorkQueue的实现，负责同步Informer和控制循环之间的数据
		recorder:        recorder,
	}

	glog.Info("Setting up student event handlers")

	//studentInformer注册了三个Handler（AddFunc、UpdateFunc和DeleteFunc）
	// 分别对应API对象的“添加”“更新”和“删除”事件。而具体的处理操作，都是将该事件对应的API对象加入到工作队列中
	studentInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: controller.enqueueMydemo,
		UpdateFunc: func(old, new interface{}) {
			oldMydemo := old.(*crdv1.Student)
			newMydemo := new.(*crdv1.Student)
			if oldMydemo.ResourceVersion == newMydemo.ResourceVersion {
				return
			}
			controller.enqueueMydemo(new)
		},
		DeleteFunc: controller.enqueueMydemoForDelete,
	})

	return controller
}

// enqueueMydemo takes a Mydemo resource and converts it into a namespace/name
// string which is then put onto the work queue. This method should *not* be
// passed resources of any type other than Mydemo.
func (c *Controller) enqueueMydemo(obj interface{}) {
	var key string
	var err error
	if key, err = cache.MetaNamespaceKeyFunc(obj); err != nil {
		runtime.HandleError(err)
		return
	}
	c.workqueue.AddRateLimited(key)
}

// enqueueMydemoForDelete takes a deleted Mydemo resource and converts it into a namespace/name
// string which is then put onto the work queue. This method should *not* be
// passed resources of any type other than Mydemo.
func (c *Controller) enqueueMydemoForDelete(obj interface{}) {
	var key string
	var err error
	key, err = cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
	if err != nil {
		runtime.HandleError(err)
		return
	}
	c.workqueue.AddRateLimited(key)
}

//runWorker是一个不断运行的方法，并且一直会调用c.processNextWorkItem从workqueue读取和读取消
func (c *Controller) runWorker() {
	for c.processNextWorkItem() {
	}
}

//从workqueue读取和读取消息
func (c *Controller) processNextWorkItem() bool {
	obj, shutdown := c.workqueue.Get()
	if shutdown {
		return false
	}
	err := func(obj interface{}) error {
		defer c.workqueue.Done(obj)
		var key string
		var ok bool
		if key, ok = obj.(string); !ok {
			c.workqueue.Forget(obj)
			runtime.HandleError(fmt.Errorf("expected string in workqueue but got %#v", obj))
			return nil
		}
		if err := c.syncHandler(key); err != nil {
			return fmt.Errorf("error syncing '%s': %s", key, err.Error())
		}
		c.workqueue.Forget(obj)
		glog.Infof("Successfully synced '%s'", key)
		return nil
	}(obj)

	if err != nil {
		runtime.HandleError(err)
		return true
	}
	return true
}

//尝试从Informer维护的缓存中拿到了它所对应的Mydemo对象
func (c *Controller) syncHandler(key string) error {
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		runtime.HandleError(fmt.Errorf("invalid resource key: %s", key))
		return nil
	}

	mydemo, err := c.studentLister.Students(namespace).Get(name)

	//从缓存中拿不到这个对象，那就意味着这个Mydemo对象的Key是通过前面的“删除”事件添加进工作队列的。
	if err != nil {
		if errors.IsNotFound(err) {
			//对应的Mydemo对象已经被删除了
			glog.Warningf("DemoCRD: %s/%s does not exist in local cache, will delete it from Mydemo ...",
				namespace, name)
			glog.Infof("[DemoCRD] Deleting mydemo: %s/%s ...", namespace, name)
			return nil
		}
		runtime.HandleError(fmt.Errorf("failed to list mydemo by: %s/%s", namespace, name))
		return err
	}
	glog.Infof("[DemoCRD] Try to process mydemo: %#v ...", mydemo)
	c.recorder.Event(mydemo, corev1.EventTypeNormal, SuccessSynced, MessageResourceSynced)
	return nil
}
func (c *Controller) Run(threadiness int, stopCh <-chan struct{}) error {
	defer runtime.HandleCrash()
	defer c.workqueue.ShutDown()

	// 记录开始日志
	glog.Info("Starting Mydemo control loop")

	glog.Info("Waiting for informer caches to sync")

	if ok := cache.WaitForCacheSync(stopCh, c.studentSynced); !ok {
		return fmt.Errorf("failed to wait for caches to sync")
	}

	glog.Info("Starting workers")
	for i := 0; i < threadiness; i++ {
		go wait.Until(c.runWorker, time.Second, stopCh)
	}

	glog.Info("Started workers")
	<-stopCh
	glog.Info("Shutting down workers")

	return nil
}
