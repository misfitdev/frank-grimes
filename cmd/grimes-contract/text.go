package main

import (
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"
)

func prototextMarshal(m proto.Message) (string, error) {
	b, err := prototext.MarshalOptions{Multiline: true, Indent: "  "}.Marshal(m)
	return string(b), err
}
