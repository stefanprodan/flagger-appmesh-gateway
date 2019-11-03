package main

import (
	"context"
	goflag "flag"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	flag "github.com/spf13/pflag"
	"k8s.io/client-go/dynamic"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog"

	"github.com/stefanprodan/appmesh-gateway/pkg/discovery"
	"github.com/stefanprodan/appmesh-gateway/pkg/envoy"
	"github.com/stefanprodan/appmesh-gateway/pkg/server"
	"github.com/stefanprodan/appmesh-gateway/pkg/signals"
)

// VERSION semantic versioning format
const VERSION = "0.3.0"

var (
	masterURL        string
	kubeConfig       string
	port             int
	namespace        string
	ads              bool
	optIn            bool
	gatewayMesh      string
	gatewayName      string
	gatewayNamespace string
)

func init() {
	pf := rootCmd.PersistentFlags()
	pf.StringVarP(&masterURL, "master", "", "", "The address of the Kubernetes API server. Overrides any value in kubeconfig. Only required if out-of-cluster.")
	pf.StringVarP(&kubeConfig, "kubeconfig", "", "", "Path to a kubeconfig. Only required if out-of-cluster.")
	pf.IntVarP(&port, "port", "p", 18000, "Envoy xDS port to listen on.")
	pf.BoolVarP(&ads, "ads", "a", true, "ADS flag forces all Envoy resources to be explicitly named in the request.")
	pf.StringVarP(&namespace, "namespace", "n", "", "Namespace to watch for Kubernetes objects, a blank value means all namespaces.")
	pf.BoolVarP(&optIn, "opt-in", "", false, "When enabled only services with the 'expose' annotation will be discoverable.")
	pf.StringVarP(&gatewayMesh, "gateway-mesh", "", "", "App Mesh mesh that this gateway belongs to.")
	cobra.MarkFlagRequired(pf, "gateway-mesh")
	pf.StringVarP(&gatewayName, "gateway-name", "", "", "Gateway Kubernetes service name.")
	cobra.MarkFlagRequired(pf, "gateway-name")
	pf.StringVarP(&gatewayNamespace, "gateway-namespace", "", "", "Gateway Kubernetes namespace.")
	cobra.MarkFlagRequired(pf, "gateway-namespace")
}

var rootCmd = &cobra.Command{
	Use:     "appmesh-gateway",
	Long:    `appmesh-gateway is responsible for exposing services outside the mesh.`,
	Version: VERSION,
	RunE:    run,
}

func main() {
	flag.CommandLine.Parse([]string{})
	pf := rootCmd.PersistentFlags()
	addKlogFlags(pf)

	rootCmd.SetArgs(os.Args[1:])
	if err := rootCmd.Execute(); err != nil {
		e := err.Error()
		fmt.Println(strings.ToUpper(e[:1]) + e[1:])
		os.Exit(1)
	}
}

func run(cmd *cobra.Command, args []string) error {
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

	vnManager := discovery.NewVirtualNodeManager(client, gatewayMesh, gatewayName, gatewayNamespace)
	if err := vnManager.CheckAccess(); err != nil {
		klog.Fatalf("the gateway can't read App Mesh objects, check RBAC, error %v", err)
	}

	vsManager := discovery.NewVirtualServiceManager(client, optIn)
	kd := discovery.NewController(client, namespace, snapshot, vsManager, vnManager)

	klog.Info("starting App Mesh discovery workers")
	kd.Run(2, stopCh)

	return nil
}

func addKlogFlags(fs *flag.FlagSet) {
	local := goflag.NewFlagSet(os.Args[0], goflag.ExitOnError)
	klog.InitFlags(local)
	local.VisitAll(func(fl *goflag.Flag) {
		fl.Name = normalizeFlag(fl.Name)
		fs.AddGoFlag(fl)
	})
}

func normalizeFlag(s string) string {
	return strings.Replace(s, "_", "-", -1)
}
