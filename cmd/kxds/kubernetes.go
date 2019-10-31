package main

import (
	"context"

	"github.com/spf13/cobra"
	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog"

	"github.com/stefanprodan/kxds/pkg/discovery"
	"github.com/stefanprodan/kxds/pkg/envoy"
	"github.com/stefanprodan/kxds/pkg/server"
	"github.com/stefanprodan/kxds/pkg/signals"
)

var masterURL string
var kubeConfig string
var port int
var namespace string
var ads bool
var portName string

func init() {
	kubeCmd.Flags().StringVarP(&masterURL, "master", "", "", "The address of the Kubernetes API server. Overrides any value in kubeconfig. Only required if out-of-cluster.")
	kubeCmd.Flags().StringVarP(&kubeConfig, "kubeconfig", "", "", "Path to a kubeconfig. Only required if out-of-cluster.")
	kubeCmd.Flags().IntVarP(&port, "port", "p", 18000, "Port to listen on.")
	kubeCmd.Flags().StringVarP(&namespace, "namespace", "", "", "Namespace to watch for Kubernetes service.")
	kubeCmd.Flags().BoolVarP(&ads, "ads", "", false, "ADS flag forces a delay in responding to streaming requests until all resources are explicitly named in the request.")
	kubeCmd.Flags().StringVarP(&portName, "port-name", "", "http", "Include Kubernetes services with this named port.")

	rootCmd.AddCommand(kubeCmd)
}

var kubeCmd = &cobra.Command{
	Use:   `kubernetes`,
	Short: "Starts Kubernetes discovery service",
	RunE:  kubeRun,
}

func kubeRun(cmd *cobra.Command, args []string) error {
	cfg, err := clientcmd.BuildConfigFromFlags(masterURL, kubeConfig)
	if err != nil {
		klog.Fatalf("error building kubeconfig: %v", err)
	}

	clientset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		klog.Fatalf("error building kubernetes clientset: %v", err)
	}

	stopCh := signals.SetupSignalHandler()
	ctx := context.Background()
	cache := envoy.NewCache(ads)
	snapshot := envoy.NewSnapshot(cache)

	klog.Infof("starting xDS server on port %d", port)
	srv := server.NewServer(port, cache)
	go srv.Serve(ctx)

	klog.Info("waiting for Envoy to connect to the xDS server")
	srv.Report()

	klog.Info("starting Kubernetes discovery workers")
	kd := discovery.NewKubernetesDiscovery(clientset, namespace, snapshot, portName)
	kd.Run(2, stopCh)

	return nil
}
