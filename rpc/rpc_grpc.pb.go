// Code generated by protoc-gen-go-grpc. DO NOT EDIT.
// versions:
// - protoc-gen-go-grpc v1.2.0
// - protoc             v3.19.4
// source: rpc/rpc.proto

package rpc

import (
	context "context"
	grpc "google.golang.org/grpc"
	codes "google.golang.org/grpc/codes"
	status "google.golang.org/grpc/status"
)

// This is a compile-time assertion to ensure that this generated file
// is compatible with the grpc package it is being compiled against.
// Requires gRPC-Go v1.32.0 or later.
const _ = grpc.SupportPackageIsVersion7

// DaemonCtlClient is the client API for DaemonCtl service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://pkg.go.dev/google.golang.org/grpc/?tab=doc#ClientConn.NewStream.
type DaemonCtlClient interface {
	// Sends the console user's username+homedir to establish connectivity
	// and initialize the daemon with correct config.toml path
	Hello(ctx context.Context, in *HelloRequest, opts ...grpc.CallOption) (*HelloResponse, error)
	// Gets the status of the daemon
	Status(ctx context.Context, in *DaemonStatusRequest, opts ...grpc.CallOption) (*DaemonStatusResponse, error)
	// Commands daemon to check connection to object store and report back
	CheckConn(ctx context.Context, in *CheckConnRequest, opts ...grpc.CallOption) (*CheckConnResponse, error)
	// Commands to synchronize config between client and daemon
	ReadDaemonConfig(ctx context.Context, in *ReadConfigRequest, opts ...grpc.CallOption) (*ReadConfigResponse, error)
	WriteToDaemonConfig(ctx context.Context, in *WriteConfigRequest, opts ...grpc.CallOption) (*WriteConfigResponse, error)
	// Backup command
	Backup(ctx context.Context, in *BackupRequest, opts ...grpc.CallOption) (*BackupResponse, error)
}

type daemonCtlClient struct {
	cc grpc.ClientConnInterface
}

func NewDaemonCtlClient(cc grpc.ClientConnInterface) DaemonCtlClient {
	return &daemonCtlClient{cc}
}

func (c *daemonCtlClient) Hello(ctx context.Context, in *HelloRequest, opts ...grpc.CallOption) (*HelloResponse, error) {
	out := new(HelloResponse)
	err := c.cc.Invoke(ctx, "/rpc.DaemonCtl/Hello", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *daemonCtlClient) Status(ctx context.Context, in *DaemonStatusRequest, opts ...grpc.CallOption) (*DaemonStatusResponse, error) {
	out := new(DaemonStatusResponse)
	err := c.cc.Invoke(ctx, "/rpc.DaemonCtl/Status", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *daemonCtlClient) CheckConn(ctx context.Context, in *CheckConnRequest, opts ...grpc.CallOption) (*CheckConnResponse, error) {
	out := new(CheckConnResponse)
	err := c.cc.Invoke(ctx, "/rpc.DaemonCtl/CheckConn", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *daemonCtlClient) ReadDaemonConfig(ctx context.Context, in *ReadConfigRequest, opts ...grpc.CallOption) (*ReadConfigResponse, error) {
	out := new(ReadConfigResponse)
	err := c.cc.Invoke(ctx, "/rpc.DaemonCtl/ReadDaemonConfig", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *daemonCtlClient) WriteToDaemonConfig(ctx context.Context, in *WriteConfigRequest, opts ...grpc.CallOption) (*WriteConfigResponse, error) {
	out := new(WriteConfigResponse)
	err := c.cc.Invoke(ctx, "/rpc.DaemonCtl/WriteToDaemonConfig", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *daemonCtlClient) Backup(ctx context.Context, in *BackupRequest, opts ...grpc.CallOption) (*BackupResponse, error) {
	out := new(BackupResponse)
	err := c.cc.Invoke(ctx, "/rpc.DaemonCtl/Backup", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// DaemonCtlServer is the server API for DaemonCtl service.
// All implementations must embed UnimplementedDaemonCtlServer
// for forward compatibility
type DaemonCtlServer interface {
	// Sends the console user's username+homedir to establish connectivity
	// and initialize the daemon with correct config.toml path
	Hello(context.Context, *HelloRequest) (*HelloResponse, error)
	// Gets the status of the daemon
	Status(context.Context, *DaemonStatusRequest) (*DaemonStatusResponse, error)
	// Commands daemon to check connection to object store and report back
	CheckConn(context.Context, *CheckConnRequest) (*CheckConnResponse, error)
	// Commands to synchronize config between client and daemon
	ReadDaemonConfig(context.Context, *ReadConfigRequest) (*ReadConfigResponse, error)
	WriteToDaemonConfig(context.Context, *WriteConfigRequest) (*WriteConfigResponse, error)
	// Backup command
	Backup(context.Context, *BackupRequest) (*BackupResponse, error)
	mustEmbedUnimplementedDaemonCtlServer()
}

// UnimplementedDaemonCtlServer must be embedded to have forward compatible implementations.
type UnimplementedDaemonCtlServer struct {
}

func (UnimplementedDaemonCtlServer) Hello(context.Context, *HelloRequest) (*HelloResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method Hello not implemented")
}
func (UnimplementedDaemonCtlServer) Status(context.Context, *DaemonStatusRequest) (*DaemonStatusResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method Status not implemented")
}
func (UnimplementedDaemonCtlServer) CheckConn(context.Context, *CheckConnRequest) (*CheckConnResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method CheckConn not implemented")
}
func (UnimplementedDaemonCtlServer) ReadDaemonConfig(context.Context, *ReadConfigRequest) (*ReadConfigResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method ReadDaemonConfig not implemented")
}
func (UnimplementedDaemonCtlServer) WriteToDaemonConfig(context.Context, *WriteConfigRequest) (*WriteConfigResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method WriteToDaemonConfig not implemented")
}
func (UnimplementedDaemonCtlServer) Backup(context.Context, *BackupRequest) (*BackupResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method Backup not implemented")
}
func (UnimplementedDaemonCtlServer) mustEmbedUnimplementedDaemonCtlServer() {}

// UnsafeDaemonCtlServer may be embedded to opt out of forward compatibility for this service.
// Use of this interface is not recommended, as added methods to DaemonCtlServer will
// result in compilation errors.
type UnsafeDaemonCtlServer interface {
	mustEmbedUnimplementedDaemonCtlServer()
}

func RegisterDaemonCtlServer(s grpc.ServiceRegistrar, srv DaemonCtlServer) {
	s.RegisterService(&DaemonCtl_ServiceDesc, srv)
}

func _DaemonCtl_Hello_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(HelloRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(DaemonCtlServer).Hello(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/rpc.DaemonCtl/Hello",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(DaemonCtlServer).Hello(ctx, req.(*HelloRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _DaemonCtl_Status_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(DaemonStatusRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(DaemonCtlServer).Status(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/rpc.DaemonCtl/Status",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(DaemonCtlServer).Status(ctx, req.(*DaemonStatusRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _DaemonCtl_CheckConn_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(CheckConnRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(DaemonCtlServer).CheckConn(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/rpc.DaemonCtl/CheckConn",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(DaemonCtlServer).CheckConn(ctx, req.(*CheckConnRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _DaemonCtl_ReadDaemonConfig_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(ReadConfigRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(DaemonCtlServer).ReadDaemonConfig(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/rpc.DaemonCtl/ReadDaemonConfig",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(DaemonCtlServer).ReadDaemonConfig(ctx, req.(*ReadConfigRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _DaemonCtl_WriteToDaemonConfig_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(WriteConfigRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(DaemonCtlServer).WriteToDaemonConfig(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/rpc.DaemonCtl/WriteToDaemonConfig",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(DaemonCtlServer).WriteToDaemonConfig(ctx, req.(*WriteConfigRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _DaemonCtl_Backup_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(BackupRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(DaemonCtlServer).Backup(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/rpc.DaemonCtl/Backup",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(DaemonCtlServer).Backup(ctx, req.(*BackupRequest))
	}
	return interceptor(ctx, in, info, handler)
}

// DaemonCtl_ServiceDesc is the grpc.ServiceDesc for DaemonCtl service.
// It's only intended for direct use with grpc.RegisterService,
// and not to be introspected or modified (even as a copy)
var DaemonCtl_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "rpc.DaemonCtl",
	HandlerType: (*DaemonCtlServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "Hello",
			Handler:    _DaemonCtl_Hello_Handler,
		},
		{
			MethodName: "Status",
			Handler:    _DaemonCtl_Status_Handler,
		},
		{
			MethodName: "CheckConn",
			Handler:    _DaemonCtl_CheckConn_Handler,
		},
		{
			MethodName: "ReadDaemonConfig",
			Handler:    _DaemonCtl_ReadDaemonConfig_Handler,
		},
		{
			MethodName: "WriteToDaemonConfig",
			Handler:    _DaemonCtl_WriteToDaemonConfig_Handler,
		},
		{
			MethodName: "Backup",
			Handler:    _DaemonCtl_Backup_Handler,
		},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "rpc/rpc.proto",
}
