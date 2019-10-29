package main

import (
	"context"

	"github.com/spf13/cobra"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog"

	"github.com/stefanprodan/kxds/pkg/discovery"
	"github.com/stefanprodan/kxds/pkg/server"
	"github.com/stefanprodan/kxds/pkg/signals"
)

var masterURL string
var kubeConfig string
var port int
var namespace string
var ads bool

func init() {
	serveCmd.Flags().StringVarP(&masterURL, "master", "", "", "The address of the Kubernetes API server. Overrides any value in kubeconfig. Only required if out-of-cluster.")
	serveCmd.Flags().StringVarP(&kubeConfig, "kubeconfig", "", "", "Path to a kubeconfig. Only required if out-of-cluster.")
	serveCmd.Flags().IntVarP(&port, "port", "p", 18000, "Port to listen on.")
	serveCmd.Flags().StringVarP(&namespace, "namespace", "", "", "Namespace to watch for Kubernetes service.")
	serveCmd.Flags().BoolVarP(&ads, "ads", "", false, "ADS flag forces a delay in responding to streaming requests until all resources are explicitly named in the request.")

	rootCmd.AddCommand(serveCmd)
}

var serveCmd = &cobra.Command{
	Use:   `serve`,
	Short: "Starts kxds server on the specified port",
	RunE:  serve,
}

func serve(cmd *cobra.Command, args []string) error {
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

	cache := discovery.NewCache(ads)

	srv := server.NewServer(port, cache)
	go srv.Serve(ctx)
	srv.Report()

	envoyConfig := discovery.NewEnvoyConfig(cache)
	controller := discovery.NewController(clientset, namespace, envoyConfig)
	go controller.Run(2, stopCh)

	<-stopCh

	return nil
}
