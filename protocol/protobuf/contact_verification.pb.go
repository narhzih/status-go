// Code generated by protoc-gen-go. DO NOT EDIT.
// source: contact_verification.proto

package protobuf

import (
	fmt "fmt"
	proto "github.com/golang/protobuf/proto"
	math "math"
)

// Reference imports to suppress errors if they are not otherwise used.
var _ = proto.Marshal
var _ = fmt.Errorf
var _ = math.Inf

// This is a compile-time assertion to ensure that this generated file
// is compatible with the proto package it is being compiled against.
// A compilation error at this line likely means your copy of the
// proto package needs to be updated.
const _ = proto.ProtoPackageIsVersion3 // please upgrade the proto package

type RequestContactVerification struct {
	Clock                uint64   `protobuf:"varint,1,opt,name=clock,proto3" json:"clock,omitempty"`
	Challenge            string   `protobuf:"bytes,3,opt,name=challenge,proto3" json:"challenge,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *RequestContactVerification) Reset()         { *m = RequestContactVerification{} }
func (m *RequestContactVerification) String() string { return proto.CompactTextString(m) }
func (*RequestContactVerification) ProtoMessage()    {}
func (*RequestContactVerification) Descriptor() ([]byte, []int) {
	return fileDescriptor_d6997df64de39454, []int{0}
}

func (m *RequestContactVerification) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_RequestContactVerification.Unmarshal(m, b)
}
func (m *RequestContactVerification) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_RequestContactVerification.Marshal(b, m, deterministic)
}
func (m *RequestContactVerification) XXX_Merge(src proto.Message) {
	xxx_messageInfo_RequestContactVerification.Merge(m, src)
}
func (m *RequestContactVerification) XXX_Size() int {
	return xxx_messageInfo_RequestContactVerification.Size(m)
}
func (m *RequestContactVerification) XXX_DiscardUnknown() {
	xxx_messageInfo_RequestContactVerification.DiscardUnknown(m)
}

var xxx_messageInfo_RequestContactVerification proto.InternalMessageInfo

func (m *RequestContactVerification) GetClock() uint64 {
	if m != nil {
		return m.Clock
	}
	return 0
}

func (m *RequestContactVerification) GetChallenge() string {
	if m != nil {
		return m.Challenge
	}
	return ""
}

type AcceptContactVerification struct {
	Clock                uint64   `protobuf:"varint,1,opt,name=clock,proto3" json:"clock,omitempty"`
	Id                   string   `protobuf:"bytes,2,opt,name=id,proto3" json:"id,omitempty"`
	Response             string   `protobuf:"bytes,3,opt,name=response,proto3" json:"response,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *AcceptContactVerification) Reset()         { *m = AcceptContactVerification{} }
func (m *AcceptContactVerification) String() string { return proto.CompactTextString(m) }
func (*AcceptContactVerification) ProtoMessage()    {}
func (*AcceptContactVerification) Descriptor() ([]byte, []int) {
	return fileDescriptor_d6997df64de39454, []int{1}
}

func (m *AcceptContactVerification) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_AcceptContactVerification.Unmarshal(m, b)
}
func (m *AcceptContactVerification) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_AcceptContactVerification.Marshal(b, m, deterministic)
}
func (m *AcceptContactVerification) XXX_Merge(src proto.Message) {
	xxx_messageInfo_AcceptContactVerification.Merge(m, src)
}
func (m *AcceptContactVerification) XXX_Size() int {
	return xxx_messageInfo_AcceptContactVerification.Size(m)
}
func (m *AcceptContactVerification) XXX_DiscardUnknown() {
	xxx_messageInfo_AcceptContactVerification.DiscardUnknown(m)
}

var xxx_messageInfo_AcceptContactVerification proto.InternalMessageInfo

func (m *AcceptContactVerification) GetClock() uint64 {
	if m != nil {
		return m.Clock
	}
	return 0
}

func (m *AcceptContactVerification) GetId() string {
	if m != nil {
		return m.Id
	}
	return ""
}

func (m *AcceptContactVerification) GetResponse() string {
	if m != nil {
		return m.Response
	}
	return ""
}

type DeclineContactVerification struct {
	Clock                uint64   `protobuf:"varint,1,opt,name=clock,proto3" json:"clock,omitempty"`
	Id                   string   `protobuf:"bytes,2,opt,name=id,proto3" json:"id,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *DeclineContactVerification) Reset()         { *m = DeclineContactVerification{} }
func (m *DeclineContactVerification) String() string { return proto.CompactTextString(m) }
func (*DeclineContactVerification) ProtoMessage()    {}
func (*DeclineContactVerification) Descriptor() ([]byte, []int) {
	return fileDescriptor_d6997df64de39454, []int{2}
}

func (m *DeclineContactVerification) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_DeclineContactVerification.Unmarshal(m, b)
}
func (m *DeclineContactVerification) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_DeclineContactVerification.Marshal(b, m, deterministic)
}
func (m *DeclineContactVerification) XXX_Merge(src proto.Message) {
	xxx_messageInfo_DeclineContactVerification.Merge(m, src)
}
func (m *DeclineContactVerification) XXX_Size() int {
	return xxx_messageInfo_DeclineContactVerification.Size(m)
}
func (m *DeclineContactVerification) XXX_DiscardUnknown() {
	xxx_messageInfo_DeclineContactVerification.DiscardUnknown(m)
}

var xxx_messageInfo_DeclineContactVerification proto.InternalMessageInfo

func (m *DeclineContactVerification) GetClock() uint64 {
	if m != nil {
		return m.Clock
	}
	return 0
}

func (m *DeclineContactVerification) GetId() string {
	if m != nil {
		return m.Id
	}
	return ""
}

type CancelContactVerification struct {
	Clock                uint64   `protobuf:"varint,1,opt,name=clock,proto3" json:"clock,omitempty"`
	Id                   string   `protobuf:"bytes,2,opt,name=id,proto3" json:"id,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *CancelContactVerification) Reset()         { *m = CancelContactVerification{} }
func (m *CancelContactVerification) String() string { return proto.CompactTextString(m) }
func (*CancelContactVerification) ProtoMessage()    {}
func (*CancelContactVerification) Descriptor() ([]byte, []int) {
	return fileDescriptor_d6997df64de39454, []int{3}
}

func (m *CancelContactVerification) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_CancelContactVerification.Unmarshal(m, b)
}
func (m *CancelContactVerification) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_CancelContactVerification.Marshal(b, m, deterministic)
}
func (m *CancelContactVerification) XXX_Merge(src proto.Message) {
	xxx_messageInfo_CancelContactVerification.Merge(m, src)
}
func (m *CancelContactVerification) XXX_Size() int {
	return xxx_messageInfo_CancelContactVerification.Size(m)
}
func (m *CancelContactVerification) XXX_DiscardUnknown() {
	xxx_messageInfo_CancelContactVerification.DiscardUnknown(m)
}

var xxx_messageInfo_CancelContactVerification proto.InternalMessageInfo

func (m *CancelContactVerification) GetClock() uint64 {
	if m != nil {
		return m.Clock
	}
	return 0
}

func (m *CancelContactVerification) GetId() string {
	if m != nil {
		return m.Id
	}
	return ""
}

func init() {
	proto.RegisterType((*RequestContactVerification)(nil), "protobuf.RequestContactVerification")
	proto.RegisterType((*AcceptContactVerification)(nil), "protobuf.AcceptContactVerification")
	proto.RegisterType((*DeclineContactVerification)(nil), "protobuf.DeclineContactVerification")
	proto.RegisterType((*CancelContactVerification)(nil), "protobuf.CancelContactVerification")
}

func init() {
	proto.RegisterFile("contact_verification.proto", fileDescriptor_d6997df64de39454)
}

var fileDescriptor_d6997df64de39454 = []byte{
	// 194 bytes of a gzipped FileDescriptorProto
	0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0xff, 0xe2, 0x92, 0x4a, 0xce, 0xcf, 0x2b,
	0x49, 0x4c, 0x2e, 0x89, 0x2f, 0x4b, 0x2d, 0xca, 0x4c, 0xcb, 0x4c, 0x4e, 0x2c, 0xc9, 0xcc, 0xcf,
	0xd3, 0x2b, 0x28, 0xca, 0x2f, 0xc9, 0x17, 0xe2, 0x00, 0x53, 0x49, 0xa5, 0x69, 0x4a, 0x01, 0x5c,
	0x52, 0x41, 0xa9, 0x85, 0xa5, 0xa9, 0xc5, 0x25, 0xce, 0x10, 0xe5, 0x61, 0x48, 0xaa, 0x85, 0x44,
	0xb8, 0x58, 0x93, 0x73, 0xf2, 0x93, 0xb3, 0x25, 0x18, 0x15, 0x18, 0x35, 0x58, 0x82, 0x20, 0x1c,
	0x21, 0x19, 0x2e, 0xce, 0xe4, 0x8c, 0xc4, 0x9c, 0x9c, 0xd4, 0xbc, 0xf4, 0x54, 0x09, 0x66, 0x05,
	0x46, 0x0d, 0xce, 0x20, 0x84, 0x80, 0x52, 0x2c, 0x97, 0xa4, 0x63, 0x72, 0x72, 0x6a, 0x01, 0x09,
	0x06, 0xf2, 0x71, 0x31, 0x65, 0xa6, 0x48, 0x30, 0x81, 0x4d, 0x62, 0xca, 0x4c, 0x11, 0x92, 0xe2,
	0xe2, 0x28, 0x4a, 0x2d, 0x2e, 0xc8, 0xcf, 0x2b, 0x86, 0x99, 0x0f, 0xe7, 0x2b, 0x39, 0x71, 0x49,
	0xb9, 0xa4, 0x26, 0xe7, 0x64, 0xe6, 0xa5, 0x92, 0x6d, 0xbe, 0x92, 0x23, 0x97, 0xa4, 0x73, 0x62,
	0x5e, 0x72, 0x6a, 0x0e, 0xd9, 0x46, 0x38, 0xf1, 0x46, 0x71, 0xeb, 0xe9, 0x5b, 0xc3, 0x82, 0x31,
	0x89, 0x0d, 0xcc, 0x32, 0x06, 0x04, 0x00, 0x00, 0xff, 0xff, 0xd4, 0x2b, 0x89, 0x8f, 0x75, 0x01,
	0x00, 0x00,
}
