package main

import (
	"context"

	"github.com/spf13/cobra"
	"k8s.io/client-go/dynamic"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog"

	"github.com/stefanprodan/kxds/pkg/discovery"
	"github.com/stefanprodan/kxds/pkg/envoy"
	"github.com/stefanprodan/kxds/pkg/server"
	"github.com/stefanprodan/kxds/pkg/signals"
)

var gatewayMesh string
var gatewayName string
var gatewayNamespace string

func init() {
	appmeshFlags := appmeshCmd.Flags()
	appmeshFlags.StringVarP(&gatewayMesh, "gateway-mesh", "", "", "App Mesh mesh that this gateway belongs to.")
	cobra.MarkFlagRequired(appmeshFlags, "gateway-mesh")
	appmeshFlags.StringVarP(&gatewayName, "gateway-name", "", "", "Gateway Kubernetes service name.")
	cobra.MarkFlagRequired(appmeshFlags, "gateway-name")
	appmeshFlags.StringVarP(&gatewayNamespace, "gateway-namespace", "", "", "Gateway Kubernetes namespace.")
	cobra.MarkFlagRequired(appmeshFlags, "gateway-namespace")

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

	client, err := dynamic.NewForConfig(cfg)
	if err != nil {
		klog.Fatalf("error building kubernetes client: %v", err)
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
	kd := discovery.NewAppmeshDiscovery(client, namespace, snapshot, gatewayMesh, gatewayName, gatewayNamespace)
	kd.Run(2, stopCh)

	return nil
}
