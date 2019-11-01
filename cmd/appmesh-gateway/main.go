package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

const VERSION = "0.3.0"

var (
	masterURL  string
	kubeConfig string
	port       int
	namespace  string
	ads        bool
	optIn      bool
)

func init() {
	pf := rootCmd.PersistentFlags()
	pf.StringVarP(&masterURL, "master", "", "", "The address of the Kubernetes API server. Overrides any value in kubeconfig. Only required if out-of-cluster.")
	pf.StringVarP(&kubeConfig, "kubeconfig", "", "", "Path to a kubeconfig. Only required if out-of-cluster.")
	pf.IntVarP(&port, "port", "p", 18000, "Envoy xDS port to listen on.")
	pf.BoolVarP(&ads, "ads", "a", true, "ADS flag forces all Envoy resources to be explicitly named in the request.")
	pf.StringVarP(&namespace, "namespace", "n", "", "Namespace to watch for Kubernetes objects, a blank value means all namespaces.")
	pf.BoolVarP(&optIn, "opt-in", "", false, "When enabled only services with the 'expose' annotation will be discoverable.")

}

var rootCmd = &cobra.Command{
	Use:     "kxds",
	Version: VERSION,
}

func main() {
	rootCmd.SetArgs(os.Args[1:])
	if err := rootCmd.Execute(); err != nil {
		e := err.Error()
		fmt.Println(strings.ToUpper(e[:1]) + e[1:])
		os.Exit(1)
	}
}
