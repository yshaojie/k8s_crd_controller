package main

import (
	"flag"
	"github.com/golang/glog"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	clientset "k8s_crd_controller/pkg/generated/clientset/versioned"
	informers "k8s_crd_controller/pkg/generated/informers/externalversions"
	"os"
	"os/signal"
	"syscall"
	"time"
)

//程序启动参数
var (
	flagSet              = flag.NewFlagSet("crddemo", flag.ExitOnError)
	master               = flag.String("master", "", "The address of the Kubernetes API server. Overrides any value in kubeconfig. Only required if out-of-cluster.")
	kubeconfig           = flag.String("kubeconfig", "", "Path to a kubeconfig. Only required if out-of-cluster.")
	onlyOneSignalHandler = make(chan struct{})
	shutdownSignals      = []os.Signal{os.Interrupt, syscall.SIGTERM}
)

//设置信号处理
func setupSignalHandler() (stopCh <-chan struct{}) {
	close(onlyOneSignalHandler)

	stop := make(chan struct{})
	c := make(chan os.Signal, 2)
	signal.Notify(c, shutdownSignals...)
	go func() {
		<-c
		close(stop)
		<-c
		os.Exit(1)
	}()

	return stop
}

func main() {
	flag.Parse()

	//设置一个信号处理，应用于优雅关闭
	stopCh := setupSignalHandler()

	cfg, err := clientcmd.BuildConfigFromFlags(*master, *kubeconfig)
	if err != nil {
		glog.Fatalf("Error building kubeconfig: %s", err.Error())
	}

	kubeClient, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		glog.Fatalf("Error building kubernetes clientset: %s", err.Error())
	}

	mydemoClient, err := clientset.NewForConfig(cfg)
	if err != nil {
		glog.Fatalf("Error building example clientset: %s", err.Error())
	}

	//informerFactory工厂类， 这里注入我们通过代码生成的client
	//clent主要用于和API Server进行通信，实现ListAndWatch
	mydemoInformerFactory := informers.NewSharedInformerFactory(mydemoClient, time.Second*30)

	//生成一个crddemo组的Mydemo对象传递给自定义控制器
	controller := NewController(kubeClient, mydemoClient,
		mydemoInformerFactory.Crd().V1().Students())

	mydemoInformerFactory.Start(stopCh)

	if err = controller.Run(2, stopCh); err != nil {
		glog.Fatalf("Error running controller: %s", err.Error())
	}
}
