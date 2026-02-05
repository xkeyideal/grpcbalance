package picker

import (
	"github.com/xkeyideal/grpcbalance/grpclient/logger"

	"google.golang.org/grpc/balancer"
	"google.golang.org/grpc/resolver"
)

// WeightAttributeKey is the key for weight attribute in gRPC resolver.Address.Attributes.
// The canonical definition is in grpclient/internal.WeightAttributeKey.
// This is kept for backward compatibility with existing code that imports picker.WeightAttributeKey.
const WeightAttributeKey = "x_customize_weight"

// PickerBuilder creates balancer.Picker.
type PickerBuilder interface {
	// Build returns a picker that will be used by gRPC to pick a SubConn.
	Build(info PickerBuildInfo) balancer.Picker
	// SetLogger sets the logger for this picker builder
	SetLogger(log logger.Logger)
}

// PickerBuildInfo contains information needed by the picker builder to
// construct a picker.
type PickerBuildInfo struct {
	// ReadySCs is a map from all ready SubConns to the Addresses used to
	// create them.
	ReadySCs map[balancer.SubConn]SubConnInfo
}

// SubConnInfo contains information about a SubConn created by the base
// balancer.
type SubConnInfo struct {
	Address resolver.Address // the address used to create this SubConn
}
