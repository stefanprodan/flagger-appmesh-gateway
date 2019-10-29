package server

import (
	"context"
	"fmt"
	"net"

	v2 "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	discovery "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v2"
	"github.com/envoyproxy/go-control-plane/pkg/cache"
	xds "github.com/envoyproxy/go-control-plane/pkg/server"
	"google.golang.org/grpc"
	"k8s.io/klog"
)

type Server struct {
	config    cache.SnapshotCache
	xdsServer xds.Server
	port      int
	cb        *callbacks
	cbSignal  chan struct{}
}

func NewServer(port int, config cache.SnapshotCache) *Server {
	cbSignal := make(chan struct{})
	cb := &callbacks{
		signal:   cbSignal,
		fetches:  0,
		requests: 0,
	}

	xdsServer := xds.NewServer(config, cb)

	return &Server{
		config:    config,
		xdsServer: xdsServer,
		port:      port,
		cbSignal:  cbSignal,
		cb:        cb,
	}
}

func (srv *Server) Serve(ctx context.Context) {
	var options []grpc.ServerOption
	options = append(options, grpc.MaxConcurrentStreams(1000000))
	grpcServer := grpc.NewServer(options...)

	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", srv.port))
	if err != nil {
		klog.Fatalf("server failed to listen on %d %v", srv.port, err)
	}

	discovery.RegisterAggregatedDiscoveryServiceServer(grpcServer, srv.xdsServer)
	v2.RegisterEndpointDiscoveryServiceServer(grpcServer, srv.xdsServer)
	v2.RegisterClusterDiscoveryServiceServer(grpcServer, srv.xdsServer)
	v2.RegisterRouteDiscoveryServiceServer(grpcServer, srv.xdsServer)
	v2.RegisterListenerDiscoveryServiceServer(grpcServer, srv.xdsServer)

	klog.Infof("server started on port %d", srv.port)
	go func() {
		if err = grpcServer.Serve(listener); err != nil {
			klog.Error(err)
		}
	}()
	<-ctx.Done()

	grpcServer.GracefulStop()
}

func (srv *Server) Report() {
	<-srv.cbSignal
	srv.cb.Report()
}
