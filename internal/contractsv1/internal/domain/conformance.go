package domain

var deviceMessageSchemas = map[string]SchemaName{
	"command": SchemaDeviceCommand,
	"receipt": SchemaDeviceReceipt,
	"result":  SchemaDeviceResult,
	"state":   SchemaDeviceState,
}

func SchemaForMessageType(messageType string) (SchemaName, bool) {
	schema, ok := deviceMessageSchemas[messageType]
	return schema, ok
}
