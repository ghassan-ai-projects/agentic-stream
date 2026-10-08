package domain

// The committed device-wire conformance frames under conformance/v1/ are data
// for the other program repositories and for tests; contractstest loads them.

var deviceMessageSchemas = map[string]SchemaName{
	"command": SchemaDeviceCommand,
	"receipt": SchemaDeviceReceipt,
	"result":  SchemaDeviceResult,
	"state":   SchemaDeviceState,
}

// SchemaForMessageType maps a device message_type to its schema name.
func SchemaForMessageType(messageType string) (SchemaName, bool) {
	schema, ok := deviceMessageSchemas[messageType]
	return schema, ok
}
