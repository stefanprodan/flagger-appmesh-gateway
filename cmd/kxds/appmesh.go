package main

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/stefanprodan/kxds/pkg/discovery"
	"k8s.io/client-go/dynamic"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog"

	"github.com/stefanprodan/kxds/pkg/envoy"
	"github.com/stefanprodan/kxds/pkg/server"
	"github.com/stefanprodan/kxds/pkg/signals"
)

var gatewayMesh string
var gatewayName string
var gatewayNamespace string

func init() {
	appmeshCmd.Flags().StringVarP(&masterURL, "master", "", "", "The address of the Kubernetes API server. Overrides any value in kubeconfig. Only required if out-of-cluster.")
	appmeshCmd.Flags().StringVarP(&kubeConfig, "kubeconfig", "", "", "Path to a kubeconfig. Only required if out-of-cluster.")
	appmeshCmd.Flags().IntVarP(&port, "port", "p", 18000, "Port to listen on.")
	appmeshCmd.Flags().StringVarP(&namespace, "namespace", "", "", "Namespace to watch for App Mesh virtual service.")
	appmeshCmd.Flags().BoolVarP(&ads, "ads", "", true, "ADS flag forces a delay in responding to streaming requests until all resources are explicitly named in the request.")
	appmeshCmd.Flags().StringVarP(&gatewayMesh, "gateway-mesh", "", "", "App Mesh gateway mesh.")
	appmeshCmd.Flags().StringVarP(&gatewayName, "gateway-name", "", "", "App Mesh gateway name.")
	appmeshCmd.Flags().StringVarP(&gatewayNamespace, "gateway-namespace", "", "", "App Mesh gateway namespace.")

	rootCmd.AddCommand(appmeshCmd)
}

var appmeshCmd = &cobra.Command{
	Use:   `appmesh`,
	Short: "Starts App Mesh discovery service",
	RunE:  appmeshRun,
}

func appmeshRun(cmd *cobra.Command, args []string) error {
	cfg, err := clientcmd.BuildConfigFromFlags(masterURL, kubeConfig)
	if err != nil {
		klog.Fatalf("error building kubeconfig: %v", err)
	}

	clientset, err := dynamic.NewForConfig(cfg)
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

	klog.Info("starting App Mesh discovery workers")
	kd := discovery.NewAppmeshDiscovery(clientset, namespace, snapshot, gatewayMesh, gatewayName, gatewayNamespace)
	kd.Run(2, stopCh)

	return nil
}
