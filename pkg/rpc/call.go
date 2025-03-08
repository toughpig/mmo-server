package rpc

import (
	"context"

	"google.golang.org/protobuf/proto"
)

// Call represents an active RPC call
type Call struct {
	ServiceMethod string        // The name of the service and method to call
	Args          proto.Message // The argument to the function
	Reply         proto.Message // The reply from the function
	Error         error         // After completion, the error status
	Done          chan *Call    // Signaled when call is complete
	ctx           context.Context
	cancelFunc    context.CancelFunc
}

// done marks the call as complete and signals the Done channel
func (call *Call) done() {
	if call.cancelFunc != nil {
		call.cancelFunc()
	}
	select {
	case call.Done <- call:
		// ok
	default:
		// We don't want to block here. It's the caller's responsibility to make
		// sure the channel has enough buffer space.
	}
}
