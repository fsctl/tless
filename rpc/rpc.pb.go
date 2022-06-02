// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.28.0
// 	protoc        v3.19.4
// source: rpc/rpc.proto

package rpc

import (
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	protoimpl "google.golang.org/protobuf/runtime/protoimpl"
	reflect "reflect"
	sync "sync"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(20 - protoimpl.MinVersion)
	// Verify that runtime/protoimpl is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(protoimpl.MaxVersion - 20)
)

type DaemonStatusResponse_State int32

const (
	DaemonStatusResponse_IDLE          DaemonStatusResponse_State = 0
	DaemonStatusResponse_CHECKING_CONN DaemonStatusResponse_State = 1
	DaemonStatusResponse_BACKING_UP    DaemonStatusResponse_State = 2
	DaemonStatusResponse_RESTORING     DaemonStatusResponse_State = 3
	DaemonStatusResponse_NEED_HELLO    DaemonStatusResponse_State = 4
)

// Enum value maps for DaemonStatusResponse_State.
var (
	DaemonStatusResponse_State_name = map[int32]string{
		0: "IDLE",
		1: "CHECKING_CONN",
		2: "BACKING_UP",
		3: "RESTORING",
		4: "NEED_HELLO",
	}
	DaemonStatusResponse_State_value = map[string]int32{
		"IDLE":          0,
		"CHECKING_CONN": 1,
		"BACKING_UP":    2,
		"RESTORING":     3,
		"NEED_HELLO":    4,
	}
)

func (x DaemonStatusResponse_State) Enum() *DaemonStatusResponse_State {
	p := new(DaemonStatusResponse_State)
	*p = x
	return p
}

func (x DaemonStatusResponse_State) String() string {
	return protoimpl.X.EnumStringOf(x.Descriptor(), protoreflect.EnumNumber(x))
}

func (DaemonStatusResponse_State) Descriptor() protoreflect.EnumDescriptor {
	return file_rpc_rpc_proto_enumTypes[0].Descriptor()
}

func (DaemonStatusResponse_State) Type() protoreflect.EnumType {
	return &file_rpc_rpc_proto_enumTypes[0]
}

func (x DaemonStatusResponse_State) Number() protoreflect.EnumNumber {
	return protoreflect.EnumNumber(x)
}

// Deprecated: Use DaemonStatusResponse_State.Descriptor instead.
func (DaemonStatusResponse_State) EnumDescriptor() ([]byte, []int) {
	return file_rpc_rpc_proto_rawDescGZIP(), []int{3, 0}
}

type CheckConnResponse_CheckConnResult int32

const (
	CheckConnResponse_SUCCESS CheckConnResponse_CheckConnResult = 0
	CheckConnResponse_ERROR   CheckConnResponse_CheckConnResult = 1
)

// Enum value maps for CheckConnResponse_CheckConnResult.
var (
	CheckConnResponse_CheckConnResult_name = map[int32]string{
		0: "SUCCESS",
		1: "ERROR",
	}
	CheckConnResponse_CheckConnResult_value = map[string]int32{
		"SUCCESS": 0,
		"ERROR":   1,
	}
)

func (x CheckConnResponse_CheckConnResult) Enum() *CheckConnResponse_CheckConnResult {
	p := new(CheckConnResponse_CheckConnResult)
	*p = x
	return p
}

func (x CheckConnResponse_CheckConnResult) String() string {
	return protoimpl.X.EnumStringOf(x.Descriptor(), protoreflect.EnumNumber(x))
}

func (CheckConnResponse_CheckConnResult) Descriptor() protoreflect.EnumDescriptor {
	return file_rpc_rpc_proto_enumTypes[1].Descriptor()
}

func (CheckConnResponse_CheckConnResult) Type() protoreflect.EnumType {
	return &file_rpc_rpc_proto_enumTypes[1]
}

func (x CheckConnResponse_CheckConnResult) Number() protoreflect.EnumNumber {
	return protoreflect.EnumNumber(x)
}

// Deprecated: Use CheckConnResponse_CheckConnResult.Descriptor instead.
func (CheckConnResponse_CheckConnResult) EnumDescriptor() ([]byte, []int) {
	return file_rpc_rpc_proto_rawDescGZIP(), []int{5, 0}
}

// Initial connect request with information about console logged-in user
type HelloRequest struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Username    string `protobuf:"bytes,1,opt,name=username,proto3" json:"username,omitempty"`
	UserHomeDir string `protobuf:"bytes,2,opt,name=userHomeDir,proto3" json:"userHomeDir,omitempty"`
}

func (x *HelloRequest) Reset() {
	*x = HelloRequest{}
	if protoimpl.UnsafeEnabled {
		mi := &file_rpc_rpc_proto_msgTypes[0]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *HelloRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*HelloRequest) ProtoMessage() {}

func (x *HelloRequest) ProtoReflect() protoreflect.Message {
	mi := &file_rpc_rpc_proto_msgTypes[0]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use HelloRequest.ProtoReflect.Descriptor instead.
func (*HelloRequest) Descriptor() ([]byte, []int) {
	return file_rpc_rpc_proto_rawDescGZIP(), []int{0}
}

func (x *HelloRequest) GetUsername() string {
	if x != nil {
		return x.Username
	}
	return ""
}

func (x *HelloRequest) GetUserHomeDir() string {
	if x != nil {
		return x.UserHomeDir
	}
	return ""
}

// TODO:  return last backup timestamp
type HelloReply struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Message string `protobuf:"bytes,1,opt,name=message,proto3" json:"message,omitempty"`
}

func (x *HelloReply) Reset() {
	*x = HelloReply{}
	if protoimpl.UnsafeEnabled {
		mi := &file_rpc_rpc_proto_msgTypes[1]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *HelloReply) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*HelloReply) ProtoMessage() {}

func (x *HelloReply) ProtoReflect() protoreflect.Message {
	mi := &file_rpc_rpc_proto_msgTypes[1]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use HelloReply.ProtoReflect.Descriptor instead.
func (*HelloReply) Descriptor() ([]byte, []int) {
	return file_rpc_rpc_proto_rawDescGZIP(), []int{1}
}

func (x *HelloReply) GetMessage() string {
	if x != nil {
		return x.Message
	}
	return ""
}

type DaemonStatusRequest struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields
}

func (x *DaemonStatusRequest) Reset() {
	*x = DaemonStatusRequest{}
	if protoimpl.UnsafeEnabled {
		mi := &file_rpc_rpc_proto_msgTypes[2]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *DaemonStatusRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*DaemonStatusRequest) ProtoMessage() {}

func (x *DaemonStatusRequest) ProtoReflect() protoreflect.Message {
	mi := &file_rpc_rpc_proto_msgTypes[2]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use DaemonStatusRequest.ProtoReflect.Descriptor instead.
func (*DaemonStatusRequest) Descriptor() ([]byte, []int) {
	return file_rpc_rpc_proto_rawDescGZIP(), []int{2}
}

type DaemonStatusResponse struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Status     DaemonStatusResponse_State `protobuf:"varint,1,opt,name=status,proto3,enum=rpc.DaemonStatusResponse_State" json:"status,omitempty"`
	Msg        string                     `protobuf:"bytes,2,opt,name=msg,proto3" json:"msg,omitempty"`
	Percentage float32                    `protobuf:"fixed32,3,opt,name=percentage,proto3" json:"percentage,omitempty"`
}

func (x *DaemonStatusResponse) Reset() {
	*x = DaemonStatusResponse{}
	if protoimpl.UnsafeEnabled {
		mi := &file_rpc_rpc_proto_msgTypes[3]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *DaemonStatusResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*DaemonStatusResponse) ProtoMessage() {}

func (x *DaemonStatusResponse) ProtoReflect() protoreflect.Message {
	mi := &file_rpc_rpc_proto_msgTypes[3]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use DaemonStatusResponse.ProtoReflect.Descriptor instead.
func (*DaemonStatusResponse) Descriptor() ([]byte, []int) {
	return file_rpc_rpc_proto_rawDescGZIP(), []int{3}
}

func (x *DaemonStatusResponse) GetStatus() DaemonStatusResponse_State {
	if x != nil {
		return x.Status
	}
	return DaemonStatusResponse_IDLE
}

func (x *DaemonStatusResponse) GetMsg() string {
	if x != nil {
		return x.Msg
	}
	return ""
}

func (x *DaemonStatusResponse) GetPercentage() float32 {
	if x != nil {
		return x.Percentage
	}
	return 0
}

type CheckConnRequest struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Endpoint   string `protobuf:"bytes,1,opt,name=Endpoint,proto3" json:"Endpoint,omitempty"`
	AccessKey  string `protobuf:"bytes,2,opt,name=AccessKey,proto3" json:"AccessKey,omitempty"`
	SecretKey  string `protobuf:"bytes,3,opt,name=SecretKey,proto3" json:"SecretKey,omitempty"`
	BucketName string `protobuf:"bytes,4,opt,name=BucketName,proto3" json:"BucketName,omitempty"`
}

func (x *CheckConnRequest) Reset() {
	*x = CheckConnRequest{}
	if protoimpl.UnsafeEnabled {
		mi := &file_rpc_rpc_proto_msgTypes[4]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *CheckConnRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*CheckConnRequest) ProtoMessage() {}

func (x *CheckConnRequest) ProtoReflect() protoreflect.Message {
	mi := &file_rpc_rpc_proto_msgTypes[4]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use CheckConnRequest.ProtoReflect.Descriptor instead.
func (*CheckConnRequest) Descriptor() ([]byte, []int) {
	return file_rpc_rpc_proto_rawDescGZIP(), []int{4}
}

func (x *CheckConnRequest) GetEndpoint() string {
	if x != nil {
		return x.Endpoint
	}
	return ""
}

func (x *CheckConnRequest) GetAccessKey() string {
	if x != nil {
		return x.AccessKey
	}
	return ""
}

func (x *CheckConnRequest) GetSecretKey() string {
	if x != nil {
		return x.SecretKey
	}
	return ""
}

func (x *CheckConnRequest) GetBucketName() string {
	if x != nil {
		return x.BucketName
	}
	return ""
}

type CheckConnResponse struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Result   CheckConnResponse_CheckConnResult `protobuf:"varint,1,opt,name=result,proto3,enum=rpc.CheckConnResponse_CheckConnResult" json:"result,omitempty"`
	ErrorMsg string                            `protobuf:"bytes,2,opt,name=errorMsg,proto3" json:"errorMsg,omitempty"`
}

func (x *CheckConnResponse) Reset() {
	*x = CheckConnResponse{}
	if protoimpl.UnsafeEnabled {
		mi := &file_rpc_rpc_proto_msgTypes[5]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *CheckConnResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*CheckConnResponse) ProtoMessage() {}

func (x *CheckConnResponse) ProtoReflect() protoreflect.Message {
	mi := &file_rpc_rpc_proto_msgTypes[5]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use CheckConnResponse.ProtoReflect.Descriptor instead.
func (*CheckConnResponse) Descriptor() ([]byte, []int) {
	return file_rpc_rpc_proto_rawDescGZIP(), []int{5}
}

func (x *CheckConnResponse) GetResult() CheckConnResponse_CheckConnResult {
	if x != nil {
		return x.Result
	}
	return CheckConnResponse_SUCCESS
}

func (x *CheckConnResponse) GetErrorMsg() string {
	if x != nil {
		return x.ErrorMsg
	}
	return ""
}

type ReadConfigRequest struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields
}

func (x *ReadConfigRequest) Reset() {
	*x = ReadConfigRequest{}
	if protoimpl.UnsafeEnabled {
		mi := &file_rpc_rpc_proto_msgTypes[6]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *ReadConfigRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*ReadConfigRequest) ProtoMessage() {}

func (x *ReadConfigRequest) ProtoReflect() protoreflect.Message {
	mi := &file_rpc_rpc_proto_msgTypes[6]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use ReadConfigRequest.ProtoReflect.Descriptor instead.
func (*ReadConfigRequest) Descriptor() ([]byte, []int) {
	return file_rpc_rpc_proto_rawDescGZIP(), []int{6}
}

type ReadConfigResponse struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Endpoint       string   `protobuf:"bytes,1,opt,name=Endpoint,proto3" json:"Endpoint,omitempty"`
	AccessKey      string   `protobuf:"bytes,2,opt,name=AccessKey,proto3" json:"AccessKey,omitempty"`
	SecretKey      string   `protobuf:"bytes,3,opt,name=SecretKey,proto3" json:"SecretKey,omitempty"`
	BucketName     string   `protobuf:"bytes,4,opt,name=BucketName,proto3" json:"BucketName,omitempty"`
	MasterPassword string   `protobuf:"bytes,5,opt,name=MasterPassword,proto3" json:"MasterPassword,omitempty"`
	Salt           string   `protobuf:"bytes,6,opt,name=Salt,proto3" json:"Salt,omitempty"`
	Dirs           []string `protobuf:"bytes,7,rep,name=Dirs,proto3" json:"Dirs,omitempty"`
	Excludes       []string `protobuf:"bytes,8,rep,name=Excludes,proto3" json:"Excludes,omitempty"`
	IsValid        bool     `protobuf:"varint,9,opt,name=IsValid,proto3" json:"IsValid,omitempty"`
	ErrMsg         string   `protobuf:"bytes,10,opt,name=ErrMsg,proto3" json:"ErrMsg,omitempty"`
}

func (x *ReadConfigResponse) Reset() {
	*x = ReadConfigResponse{}
	if protoimpl.UnsafeEnabled {
		mi := &file_rpc_rpc_proto_msgTypes[7]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *ReadConfigResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*ReadConfigResponse) ProtoMessage() {}

func (x *ReadConfigResponse) ProtoReflect() protoreflect.Message {
	mi := &file_rpc_rpc_proto_msgTypes[7]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use ReadConfigResponse.ProtoReflect.Descriptor instead.
func (*ReadConfigResponse) Descriptor() ([]byte, []int) {
	return file_rpc_rpc_proto_rawDescGZIP(), []int{7}
}

func (x *ReadConfigResponse) GetEndpoint() string {
	if x != nil {
		return x.Endpoint
	}
	return ""
}

func (x *ReadConfigResponse) GetAccessKey() string {
	if x != nil {
		return x.AccessKey
	}
	return ""
}

func (x *ReadConfigResponse) GetSecretKey() string {
	if x != nil {
		return x.SecretKey
	}
	return ""
}

func (x *ReadConfigResponse) GetBucketName() string {
	if x != nil {
		return x.BucketName
	}
	return ""
}

func (x *ReadConfigResponse) GetMasterPassword() string {
	if x != nil {
		return x.MasterPassword
	}
	return ""
}

func (x *ReadConfigResponse) GetSalt() string {
	if x != nil {
		return x.Salt
	}
	return ""
}

func (x *ReadConfigResponse) GetDirs() []string {
	if x != nil {
		return x.Dirs
	}
	return nil
}

func (x *ReadConfigResponse) GetExcludes() []string {
	if x != nil {
		return x.Excludes
	}
	return nil
}

func (x *ReadConfigResponse) GetIsValid() bool {
	if x != nil {
		return x.IsValid
	}
	return false
}

func (x *ReadConfigResponse) GetErrMsg() string {
	if x != nil {
		return x.ErrMsg
	}
	return ""
}

type WriteConfigRequest struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Endpoint       string   `protobuf:"bytes,1,opt,name=Endpoint,proto3" json:"Endpoint,omitempty"`
	AccessKey      string   `protobuf:"bytes,2,opt,name=AccessKey,proto3" json:"AccessKey,omitempty"`
	SecretKey      string   `protobuf:"bytes,3,opt,name=SecretKey,proto3" json:"SecretKey,omitempty"`
	BucketName     string   `protobuf:"bytes,4,opt,name=BucketName,proto3" json:"BucketName,omitempty"`
	MasterPassword string   `protobuf:"bytes,5,opt,name=MasterPassword,proto3" json:"MasterPassword,omitempty"`
	Salt           string   `protobuf:"bytes,6,opt,name=Salt,proto3" json:"Salt,omitempty"`
	Dirs           []string `protobuf:"bytes,7,rep,name=Dirs,proto3" json:"Dirs,omitempty"`
	Excludes       []string `protobuf:"bytes,8,rep,name=Excludes,proto3" json:"Excludes,omitempty"`
}

func (x *WriteConfigRequest) Reset() {
	*x = WriteConfigRequest{}
	if protoimpl.UnsafeEnabled {
		mi := &file_rpc_rpc_proto_msgTypes[8]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *WriteConfigRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*WriteConfigRequest) ProtoMessage() {}

func (x *WriteConfigRequest) ProtoReflect() protoreflect.Message {
	mi := &file_rpc_rpc_proto_msgTypes[8]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use WriteConfigRequest.ProtoReflect.Descriptor instead.
func (*WriteConfigRequest) Descriptor() ([]byte, []int) {
	return file_rpc_rpc_proto_rawDescGZIP(), []int{8}
}

func (x *WriteConfigRequest) GetEndpoint() string {
	if x != nil {
		return x.Endpoint
	}
	return ""
}

func (x *WriteConfigRequest) GetAccessKey() string {
	if x != nil {
		return x.AccessKey
	}
	return ""
}

func (x *WriteConfigRequest) GetSecretKey() string {
	if x != nil {
		return x.SecretKey
	}
	return ""
}

func (x *WriteConfigRequest) GetBucketName() string {
	if x != nil {
		return x.BucketName
	}
	return ""
}

func (x *WriteConfigRequest) GetMasterPassword() string {
	if x != nil {
		return x.MasterPassword
	}
	return ""
}

func (x *WriteConfigRequest) GetSalt() string {
	if x != nil {
		return x.Salt
	}
	return ""
}

func (x *WriteConfigRequest) GetDirs() []string {
	if x != nil {
		return x.Dirs
	}
	return nil
}

func (x *WriteConfigRequest) GetExcludes() []string {
	if x != nil {
		return x.Excludes
	}
	return nil
}

type WriteConfigResponse struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	DidSucceed bool   `protobuf:"varint,1,opt,name=DidSucceed,proto3" json:"DidSucceed,omitempty"`
	ErrMsg     string `protobuf:"bytes,2,opt,name=ErrMsg,proto3" json:"ErrMsg,omitempty"`
}

func (x *WriteConfigResponse) Reset() {
	*x = WriteConfigResponse{}
	if protoimpl.UnsafeEnabled {
		mi := &file_rpc_rpc_proto_msgTypes[9]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *WriteConfigResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*WriteConfigResponse) ProtoMessage() {}

func (x *WriteConfigResponse) ProtoReflect() protoreflect.Message {
	mi := &file_rpc_rpc_proto_msgTypes[9]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use WriteConfigResponse.ProtoReflect.Descriptor instead.
func (*WriteConfigResponse) Descriptor() ([]byte, []int) {
	return file_rpc_rpc_proto_rawDescGZIP(), []int{9}
}

func (x *WriteConfigResponse) GetDidSucceed() bool {
	if x != nil {
		return x.DidSucceed
	}
	return false
}

func (x *WriteConfigResponse) GetErrMsg() string {
	if x != nil {
		return x.ErrMsg
	}
	return ""
}

var File_rpc_rpc_proto protoreflect.FileDescriptor

var file_rpc_rpc_proto_rawDesc = []byte{
	0x0a, 0x0d, 0x72, 0x70, 0x63, 0x2f, 0x72, 0x70, 0x63, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x12,
	0x03, 0x72, 0x70, 0x63, 0x22, 0x4c, 0x0a, 0x0c, 0x48, 0x65, 0x6c, 0x6c, 0x6f, 0x52, 0x65, 0x71,
	0x75, 0x65, 0x73, 0x74, 0x12, 0x1a, 0x0a, 0x08, 0x75, 0x73, 0x65, 0x72, 0x6e, 0x61, 0x6d, 0x65,
	0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x08, 0x75, 0x73, 0x65, 0x72, 0x6e, 0x61, 0x6d, 0x65,
	0x12, 0x20, 0x0a, 0x0b, 0x75, 0x73, 0x65, 0x72, 0x48, 0x6f, 0x6d, 0x65, 0x44, 0x69, 0x72, 0x18,
	0x02, 0x20, 0x01, 0x28, 0x09, 0x52, 0x0b, 0x75, 0x73, 0x65, 0x72, 0x48, 0x6f, 0x6d, 0x65, 0x44,
	0x69, 0x72, 0x22, 0x26, 0x0a, 0x0a, 0x48, 0x65, 0x6c, 0x6c, 0x6f, 0x52, 0x65, 0x70, 0x6c, 0x79,
	0x12, 0x18, 0x0a, 0x07, 0x6d, 0x65, 0x73, 0x73, 0x61, 0x67, 0x65, 0x18, 0x01, 0x20, 0x01, 0x28,
	0x09, 0x52, 0x07, 0x6d, 0x65, 0x73, 0x73, 0x61, 0x67, 0x65, 0x22, 0x15, 0x0a, 0x13, 0x44, 0x61,
	0x65, 0x6d, 0x6f, 0x6e, 0x53, 0x74, 0x61, 0x74, 0x75, 0x73, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73,
	0x74, 0x22, 0xd6, 0x01, 0x0a, 0x14, 0x44, 0x61, 0x65, 0x6d, 0x6f, 0x6e, 0x53, 0x74, 0x61, 0x74,
	0x75, 0x73, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x12, 0x37, 0x0a, 0x06, 0x73, 0x74,
	0x61, 0x74, 0x75, 0x73, 0x18, 0x01, 0x20, 0x01, 0x28, 0x0e, 0x32, 0x1f, 0x2e, 0x72, 0x70, 0x63,
	0x2e, 0x44, 0x61, 0x65, 0x6d, 0x6f, 0x6e, 0x53, 0x74, 0x61, 0x74, 0x75, 0x73, 0x52, 0x65, 0x73,
	0x70, 0x6f, 0x6e, 0x73, 0x65, 0x2e, 0x53, 0x74, 0x61, 0x74, 0x65, 0x52, 0x06, 0x73, 0x74, 0x61,
	0x74, 0x75, 0x73, 0x12, 0x10, 0x0a, 0x03, 0x6d, 0x73, 0x67, 0x18, 0x02, 0x20, 0x01, 0x28, 0x09,
	0x52, 0x03, 0x6d, 0x73, 0x67, 0x12, 0x1e, 0x0a, 0x0a, 0x70, 0x65, 0x72, 0x63, 0x65, 0x6e, 0x74,
	0x61, 0x67, 0x65, 0x18, 0x03, 0x20, 0x01, 0x28, 0x02, 0x52, 0x0a, 0x70, 0x65, 0x72, 0x63, 0x65,
	0x6e, 0x74, 0x61, 0x67, 0x65, 0x22, 0x53, 0x0a, 0x05, 0x53, 0x74, 0x61, 0x74, 0x65, 0x12, 0x08,
	0x0a, 0x04, 0x49, 0x44, 0x4c, 0x45, 0x10, 0x00, 0x12, 0x11, 0x0a, 0x0d, 0x43, 0x48, 0x45, 0x43,
	0x4b, 0x49, 0x4e, 0x47, 0x5f, 0x43, 0x4f, 0x4e, 0x4e, 0x10, 0x01, 0x12, 0x0e, 0x0a, 0x0a, 0x42,
	0x41, 0x43, 0x4b, 0x49, 0x4e, 0x47, 0x5f, 0x55, 0x50, 0x10, 0x02, 0x12, 0x0d, 0x0a, 0x09, 0x52,
	0x45, 0x53, 0x54, 0x4f, 0x52, 0x49, 0x4e, 0x47, 0x10, 0x03, 0x12, 0x0e, 0x0a, 0x0a, 0x4e, 0x45,
	0x45, 0x44, 0x5f, 0x48, 0x45, 0x4c, 0x4c, 0x4f, 0x10, 0x04, 0x22, 0x8a, 0x01, 0x0a, 0x10, 0x43,
	0x68, 0x65, 0x63, 0x6b, 0x43, 0x6f, 0x6e, 0x6e, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x12,
	0x1a, 0x0a, 0x08, 0x45, 0x6e, 0x64, 0x70, 0x6f, 0x69, 0x6e, 0x74, 0x18, 0x01, 0x20, 0x01, 0x28,
	0x09, 0x52, 0x08, 0x45, 0x6e, 0x64, 0x70, 0x6f, 0x69, 0x6e, 0x74, 0x12, 0x1c, 0x0a, 0x09, 0x41,
	0x63, 0x63, 0x65, 0x73, 0x73, 0x4b, 0x65, 0x79, 0x18, 0x02, 0x20, 0x01, 0x28, 0x09, 0x52, 0x09,
	0x41, 0x63, 0x63, 0x65, 0x73, 0x73, 0x4b, 0x65, 0x79, 0x12, 0x1c, 0x0a, 0x09, 0x53, 0x65, 0x63,
	0x72, 0x65, 0x74, 0x4b, 0x65, 0x79, 0x18, 0x03, 0x20, 0x01, 0x28, 0x09, 0x52, 0x09, 0x53, 0x65,
	0x63, 0x72, 0x65, 0x74, 0x4b, 0x65, 0x79, 0x12, 0x1e, 0x0a, 0x0a, 0x42, 0x75, 0x63, 0x6b, 0x65,
	0x74, 0x4e, 0x61, 0x6d, 0x65, 0x18, 0x04, 0x20, 0x01, 0x28, 0x09, 0x52, 0x0a, 0x42, 0x75, 0x63,
	0x6b, 0x65, 0x74, 0x4e, 0x61, 0x6d, 0x65, 0x22, 0x9a, 0x01, 0x0a, 0x11, 0x43, 0x68, 0x65, 0x63,
	0x6b, 0x43, 0x6f, 0x6e, 0x6e, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x12, 0x3e, 0x0a,
	0x06, 0x72, 0x65, 0x73, 0x75, 0x6c, 0x74, 0x18, 0x01, 0x20, 0x01, 0x28, 0x0e, 0x32, 0x26, 0x2e,
	0x72, 0x70, 0x63, 0x2e, 0x43, 0x68, 0x65, 0x63, 0x6b, 0x43, 0x6f, 0x6e, 0x6e, 0x52, 0x65, 0x73,
	0x70, 0x6f, 0x6e, 0x73, 0x65, 0x2e, 0x43, 0x68, 0x65, 0x63, 0x6b, 0x43, 0x6f, 0x6e, 0x6e, 0x52,
	0x65, 0x73, 0x75, 0x6c, 0x74, 0x52, 0x06, 0x72, 0x65, 0x73, 0x75, 0x6c, 0x74, 0x12, 0x1a, 0x0a,
	0x08, 0x65, 0x72, 0x72, 0x6f, 0x72, 0x4d, 0x73, 0x67, 0x18, 0x02, 0x20, 0x01, 0x28, 0x09, 0x52,
	0x08, 0x65, 0x72, 0x72, 0x6f, 0x72, 0x4d, 0x73, 0x67, 0x22, 0x29, 0x0a, 0x0f, 0x43, 0x68, 0x65,
	0x63, 0x6b, 0x43, 0x6f, 0x6e, 0x6e, 0x52, 0x65, 0x73, 0x75, 0x6c, 0x74, 0x12, 0x0b, 0x0a, 0x07,
	0x53, 0x55, 0x43, 0x43, 0x45, 0x53, 0x53, 0x10, 0x00, 0x12, 0x09, 0x0a, 0x05, 0x45, 0x52, 0x52,
	0x4f, 0x52, 0x10, 0x01, 0x22, 0x13, 0x0a, 0x11, 0x52, 0x65, 0x61, 0x64, 0x43, 0x6f, 0x6e, 0x66,
	0x69, 0x67, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x22, 0xaa, 0x02, 0x0a, 0x12, 0x52, 0x65,
	0x61, 0x64, 0x43, 0x6f, 0x6e, 0x66, 0x69, 0x67, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65,
	0x12, 0x1a, 0x0a, 0x08, 0x45, 0x6e, 0x64, 0x70, 0x6f, 0x69, 0x6e, 0x74, 0x18, 0x01, 0x20, 0x01,
	0x28, 0x09, 0x52, 0x08, 0x45, 0x6e, 0x64, 0x70, 0x6f, 0x69, 0x6e, 0x74, 0x12, 0x1c, 0x0a, 0x09,
	0x41, 0x63, 0x63, 0x65, 0x73, 0x73, 0x4b, 0x65, 0x79, 0x18, 0x02, 0x20, 0x01, 0x28, 0x09, 0x52,
	0x09, 0x41, 0x63, 0x63, 0x65, 0x73, 0x73, 0x4b, 0x65, 0x79, 0x12, 0x1c, 0x0a, 0x09, 0x53, 0x65,
	0x63, 0x72, 0x65, 0x74, 0x4b, 0x65, 0x79, 0x18, 0x03, 0x20, 0x01, 0x28, 0x09, 0x52, 0x09, 0x53,
	0x65, 0x63, 0x72, 0x65, 0x74, 0x4b, 0x65, 0x79, 0x12, 0x1e, 0x0a, 0x0a, 0x42, 0x75, 0x63, 0x6b,
	0x65, 0x74, 0x4e, 0x61, 0x6d, 0x65, 0x18, 0x04, 0x20, 0x01, 0x28, 0x09, 0x52, 0x0a, 0x42, 0x75,
	0x63, 0x6b, 0x65, 0x74, 0x4e, 0x61, 0x6d, 0x65, 0x12, 0x26, 0x0a, 0x0e, 0x4d, 0x61, 0x73, 0x74,
	0x65, 0x72, 0x50, 0x61, 0x73, 0x73, 0x77, 0x6f, 0x72, 0x64, 0x18, 0x05, 0x20, 0x01, 0x28, 0x09,
	0x52, 0x0e, 0x4d, 0x61, 0x73, 0x74, 0x65, 0x72, 0x50, 0x61, 0x73, 0x73, 0x77, 0x6f, 0x72, 0x64,
	0x12, 0x12, 0x0a, 0x04, 0x53, 0x61, 0x6c, 0x74, 0x18, 0x06, 0x20, 0x01, 0x28, 0x09, 0x52, 0x04,
	0x53, 0x61, 0x6c, 0x74, 0x12, 0x12, 0x0a, 0x04, 0x44, 0x69, 0x72, 0x73, 0x18, 0x07, 0x20, 0x03,
	0x28, 0x09, 0x52, 0x04, 0x44, 0x69, 0x72, 0x73, 0x12, 0x1a, 0x0a, 0x08, 0x45, 0x78, 0x63, 0x6c,
	0x75, 0x64, 0x65, 0x73, 0x18, 0x08, 0x20, 0x03, 0x28, 0x09, 0x52, 0x08, 0x45, 0x78, 0x63, 0x6c,
	0x75, 0x64, 0x65, 0x73, 0x12, 0x18, 0x0a, 0x07, 0x49, 0x73, 0x56, 0x61, 0x6c, 0x69, 0x64, 0x18,
	0x09, 0x20, 0x01, 0x28, 0x08, 0x52, 0x07, 0x49, 0x73, 0x56, 0x61, 0x6c, 0x69, 0x64, 0x12, 0x16,
	0x0a, 0x06, 0x45, 0x72, 0x72, 0x4d, 0x73, 0x67, 0x18, 0x0a, 0x20, 0x01, 0x28, 0x09, 0x52, 0x06,
	0x45, 0x72, 0x72, 0x4d, 0x73, 0x67, 0x22, 0xf8, 0x01, 0x0a, 0x12, 0x57, 0x72, 0x69, 0x74, 0x65,
	0x43, 0x6f, 0x6e, 0x66, 0x69, 0x67, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x12, 0x1a, 0x0a,
	0x08, 0x45, 0x6e, 0x64, 0x70, 0x6f, 0x69, 0x6e, 0x74, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52,
	0x08, 0x45, 0x6e, 0x64, 0x70, 0x6f, 0x69, 0x6e, 0x74, 0x12, 0x1c, 0x0a, 0x09, 0x41, 0x63, 0x63,
	0x65, 0x73, 0x73, 0x4b, 0x65, 0x79, 0x18, 0x02, 0x20, 0x01, 0x28, 0x09, 0x52, 0x09, 0x41, 0x63,
	0x63, 0x65, 0x73, 0x73, 0x4b, 0x65, 0x79, 0x12, 0x1c, 0x0a, 0x09, 0x53, 0x65, 0x63, 0x72, 0x65,
	0x74, 0x4b, 0x65, 0x79, 0x18, 0x03, 0x20, 0x01, 0x28, 0x09, 0x52, 0x09, 0x53, 0x65, 0x63, 0x72,
	0x65, 0x74, 0x4b, 0x65, 0x79, 0x12, 0x1e, 0x0a, 0x0a, 0x42, 0x75, 0x63, 0x6b, 0x65, 0x74, 0x4e,
	0x61, 0x6d, 0x65, 0x18, 0x04, 0x20, 0x01, 0x28, 0x09, 0x52, 0x0a, 0x42, 0x75, 0x63, 0x6b, 0x65,
	0x74, 0x4e, 0x61, 0x6d, 0x65, 0x12, 0x26, 0x0a, 0x0e, 0x4d, 0x61, 0x73, 0x74, 0x65, 0x72, 0x50,
	0x61, 0x73, 0x73, 0x77, 0x6f, 0x72, 0x64, 0x18, 0x05, 0x20, 0x01, 0x28, 0x09, 0x52, 0x0e, 0x4d,
	0x61, 0x73, 0x74, 0x65, 0x72, 0x50, 0x61, 0x73, 0x73, 0x77, 0x6f, 0x72, 0x64, 0x12, 0x12, 0x0a,
	0x04, 0x53, 0x61, 0x6c, 0x74, 0x18, 0x06, 0x20, 0x01, 0x28, 0x09, 0x52, 0x04, 0x53, 0x61, 0x6c,
	0x74, 0x12, 0x12, 0x0a, 0x04, 0x44, 0x69, 0x72, 0x73, 0x18, 0x07, 0x20, 0x03, 0x28, 0x09, 0x52,
	0x04, 0x44, 0x69, 0x72, 0x73, 0x12, 0x1a, 0x0a, 0x08, 0x45, 0x78, 0x63, 0x6c, 0x75, 0x64, 0x65,
	0x73, 0x18, 0x08, 0x20, 0x03, 0x28, 0x09, 0x52, 0x08, 0x45, 0x78, 0x63, 0x6c, 0x75, 0x64, 0x65,
	0x73, 0x22, 0x4d, 0x0a, 0x13, 0x57, 0x72, 0x69, 0x74, 0x65, 0x43, 0x6f, 0x6e, 0x66, 0x69, 0x67,
	0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x12, 0x1e, 0x0a, 0x0a, 0x44, 0x69, 0x64, 0x53,
	0x75, 0x63, 0x63, 0x65, 0x65, 0x64, 0x18, 0x01, 0x20, 0x01, 0x28, 0x08, 0x52, 0x0a, 0x44, 0x69,
	0x64, 0x53, 0x75, 0x63, 0x63, 0x65, 0x65, 0x64, 0x12, 0x16, 0x0a, 0x06, 0x45, 0x72, 0x72, 0x4d,
	0x73, 0x67, 0x18, 0x02, 0x20, 0x01, 0x28, 0x09, 0x52, 0x06, 0x45, 0x72, 0x72, 0x4d, 0x73, 0x67,
	0x32, 0xcc, 0x02, 0x0a, 0x09, 0x44, 0x61, 0x65, 0x6d, 0x6f, 0x6e, 0x43, 0x74, 0x6c, 0x12, 0x2d,
	0x0a, 0x05, 0x48, 0x65, 0x6c, 0x6c, 0x6f, 0x12, 0x11, 0x2e, 0x72, 0x70, 0x63, 0x2e, 0x48, 0x65,
	0x6c, 0x6c, 0x6f, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x1a, 0x0f, 0x2e, 0x72, 0x70, 0x63,
	0x2e, 0x48, 0x65, 0x6c, 0x6c, 0x6f, 0x52, 0x65, 0x70, 0x6c, 0x79, 0x22, 0x00, 0x12, 0x3f, 0x0a,
	0x06, 0x53, 0x74, 0x61, 0x74, 0x75, 0x73, 0x12, 0x18, 0x2e, 0x72, 0x70, 0x63, 0x2e, 0x44, 0x61,
	0x65, 0x6d, 0x6f, 0x6e, 0x53, 0x74, 0x61, 0x74, 0x75, 0x73, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73,
	0x74, 0x1a, 0x19, 0x2e, 0x72, 0x70, 0x63, 0x2e, 0x44, 0x61, 0x65, 0x6d, 0x6f, 0x6e, 0x53, 0x74,
	0x61, 0x74, 0x75, 0x73, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x22, 0x00, 0x12, 0x3c,
	0x0a, 0x09, 0x43, 0x68, 0x65, 0x63, 0x6b, 0x43, 0x6f, 0x6e, 0x6e, 0x12, 0x15, 0x2e, 0x72, 0x70,
	0x63, 0x2e, 0x43, 0x68, 0x65, 0x63, 0x6b, 0x43, 0x6f, 0x6e, 0x6e, 0x52, 0x65, 0x71, 0x75, 0x65,
	0x73, 0x74, 0x1a, 0x16, 0x2e, 0x72, 0x70, 0x63, 0x2e, 0x43, 0x68, 0x65, 0x63, 0x6b, 0x43, 0x6f,
	0x6e, 0x6e, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x22, 0x00, 0x12, 0x45, 0x0a, 0x10,
	0x52, 0x65, 0x61, 0x64, 0x44, 0x61, 0x65, 0x6d, 0x6f, 0x6e, 0x43, 0x6f, 0x6e, 0x66, 0x69, 0x67,
	0x12, 0x16, 0x2e, 0x72, 0x70, 0x63, 0x2e, 0x52, 0x65, 0x61, 0x64, 0x43, 0x6f, 0x6e, 0x66, 0x69,
	0x67, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x1a, 0x17, 0x2e, 0x72, 0x70, 0x63, 0x2e, 0x52,
	0x65, 0x61, 0x64, 0x43, 0x6f, 0x6e, 0x66, 0x69, 0x67, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73,
	0x65, 0x22, 0x00, 0x12, 0x4a, 0x0a, 0x13, 0x57, 0x72, 0x69, 0x74, 0x65, 0x54, 0x6f, 0x44, 0x61,
	0x65, 0x6d, 0x6f, 0x6e, 0x43, 0x6f, 0x6e, 0x66, 0x69, 0x67, 0x12, 0x17, 0x2e, 0x72, 0x70, 0x63,
	0x2e, 0x57, 0x72, 0x69, 0x74, 0x65, 0x43, 0x6f, 0x6e, 0x66, 0x69, 0x67, 0x52, 0x65, 0x71, 0x75,
	0x65, 0x73, 0x74, 0x1a, 0x18, 0x2e, 0x72, 0x70, 0x63, 0x2e, 0x57, 0x72, 0x69, 0x74, 0x65, 0x43,
	0x6f, 0x6e, 0x66, 0x69, 0x67, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x22, 0x00, 0x42,
	0x1c, 0x5a, 0x1a, 0x67, 0x69, 0x74, 0x68, 0x75, 0x62, 0x2e, 0x63, 0x6f, 0x6d, 0x2f, 0x66, 0x73,
	0x63, 0x74, 0x6c, 0x2f, 0x74, 0x6c, 0x65, 0x73, 0x73, 0x2f, 0x72, 0x70, 0x63, 0x62, 0x06, 0x70,
	0x72, 0x6f, 0x74, 0x6f, 0x33,
}

var (
	file_rpc_rpc_proto_rawDescOnce sync.Once
	file_rpc_rpc_proto_rawDescData = file_rpc_rpc_proto_rawDesc
)

func file_rpc_rpc_proto_rawDescGZIP() []byte {
	file_rpc_rpc_proto_rawDescOnce.Do(func() {
		file_rpc_rpc_proto_rawDescData = protoimpl.X.CompressGZIP(file_rpc_rpc_proto_rawDescData)
	})
	return file_rpc_rpc_proto_rawDescData
}

var file_rpc_rpc_proto_enumTypes = make([]protoimpl.EnumInfo, 2)
var file_rpc_rpc_proto_msgTypes = make([]protoimpl.MessageInfo, 10)
var file_rpc_rpc_proto_goTypes = []interface{}{
	(DaemonStatusResponse_State)(0),        // 0: rpc.DaemonStatusResponse.State
	(CheckConnResponse_CheckConnResult)(0), // 1: rpc.CheckConnResponse.CheckConnResult
	(*HelloRequest)(nil),                   // 2: rpc.HelloRequest
	(*HelloReply)(nil),                     // 3: rpc.HelloReply
	(*DaemonStatusRequest)(nil),            // 4: rpc.DaemonStatusRequest
	(*DaemonStatusResponse)(nil),           // 5: rpc.DaemonStatusResponse
	(*CheckConnRequest)(nil),               // 6: rpc.CheckConnRequest
	(*CheckConnResponse)(nil),              // 7: rpc.CheckConnResponse
	(*ReadConfigRequest)(nil),              // 8: rpc.ReadConfigRequest
	(*ReadConfigResponse)(nil),             // 9: rpc.ReadConfigResponse
	(*WriteConfigRequest)(nil),             // 10: rpc.WriteConfigRequest
	(*WriteConfigResponse)(nil),            // 11: rpc.WriteConfigResponse
}
var file_rpc_rpc_proto_depIdxs = []int32{
	0,  // 0: rpc.DaemonStatusResponse.status:type_name -> rpc.DaemonStatusResponse.State
	1,  // 1: rpc.CheckConnResponse.result:type_name -> rpc.CheckConnResponse.CheckConnResult
	2,  // 2: rpc.DaemonCtl.Hello:input_type -> rpc.HelloRequest
	4,  // 3: rpc.DaemonCtl.Status:input_type -> rpc.DaemonStatusRequest
	6,  // 4: rpc.DaemonCtl.CheckConn:input_type -> rpc.CheckConnRequest
	8,  // 5: rpc.DaemonCtl.ReadDaemonConfig:input_type -> rpc.ReadConfigRequest
	10, // 6: rpc.DaemonCtl.WriteToDaemonConfig:input_type -> rpc.WriteConfigRequest
	3,  // 7: rpc.DaemonCtl.Hello:output_type -> rpc.HelloReply
	5,  // 8: rpc.DaemonCtl.Status:output_type -> rpc.DaemonStatusResponse
	7,  // 9: rpc.DaemonCtl.CheckConn:output_type -> rpc.CheckConnResponse
	9,  // 10: rpc.DaemonCtl.ReadDaemonConfig:output_type -> rpc.ReadConfigResponse
	11, // 11: rpc.DaemonCtl.WriteToDaemonConfig:output_type -> rpc.WriteConfigResponse
	7,  // [7:12] is the sub-list for method output_type
	2,  // [2:7] is the sub-list for method input_type
	2,  // [2:2] is the sub-list for extension type_name
	2,  // [2:2] is the sub-list for extension extendee
	0,  // [0:2] is the sub-list for field type_name
}

func init() { file_rpc_rpc_proto_init() }
func file_rpc_rpc_proto_init() {
	if File_rpc_rpc_proto != nil {
		return
	}
	if !protoimpl.UnsafeEnabled {
		file_rpc_rpc_proto_msgTypes[0].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*HelloRequest); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_rpc_rpc_proto_msgTypes[1].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*HelloReply); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_rpc_rpc_proto_msgTypes[2].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*DaemonStatusRequest); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_rpc_rpc_proto_msgTypes[3].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*DaemonStatusResponse); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_rpc_rpc_proto_msgTypes[4].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*CheckConnRequest); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_rpc_rpc_proto_msgTypes[5].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*CheckConnResponse); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_rpc_rpc_proto_msgTypes[6].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*ReadConfigRequest); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_rpc_rpc_proto_msgTypes[7].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*ReadConfigResponse); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_rpc_rpc_proto_msgTypes[8].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*WriteConfigRequest); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_rpc_rpc_proto_msgTypes[9].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*WriteConfigResponse); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: file_rpc_rpc_proto_rawDesc,
			NumEnums:      2,
			NumMessages:   10,
			NumExtensions: 0,
			NumServices:   1,
		},
		GoTypes:           file_rpc_rpc_proto_goTypes,
		DependencyIndexes: file_rpc_rpc_proto_depIdxs,
		EnumInfos:         file_rpc_rpc_proto_enumTypes,
		MessageInfos:      file_rpc_rpc_proto_msgTypes,
	}.Build()
	File_rpc_rpc_proto = out.File
	file_rpc_rpc_proto_rawDesc = nil
	file_rpc_rpc_proto_goTypes = nil
	file_rpc_rpc_proto_depIdxs = nil
}
