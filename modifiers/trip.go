package modifiers

import (
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

func ModifyTripStats(resp interface{}) {
	msg, ok := resp.(proto.Message)
	if !ok {
		return
	}

	v := msg.ProtoReflect()
	dataField := v.Descriptor().Fields().ByName("data")
	if dataField == nil {
		return
	}

	data := v.Mutable(dataField).Message()

	setField := func(name string, val int64) {
		f := data.Descriptor().Fields().ByName(protoreflect.Name(name))
		if f != nil {
			data.Set(f, protoreflect.ValueOfInt64(val))
		}
	}

	setField("acceptedTrips", 5000)
	setField("canceledTrips", 23000)
	setField("ongoingTrips", 3000)
	setField("scheduledTrips", 8000)
	setField("completedTrips", 16000)
	setField("pendingRequests", 10000)
}
