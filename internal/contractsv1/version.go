package contractsv1

// ContractVersion is the shared JSON contract package major version.
const ContractVersion = "situation-runtime-contracts/v1"

// ProtocolVersion is the worker transport protocol major version.
const ProtocolVersion = "agenticstream.runtime/v1"

// DeviceProtocolVersion is the version of the snake_case device wire records
// owned by this repository and consumed by the gateway, emulator, and
// firmware. Device protocol compatibility is exact: a newer or older wire
// version must not be guessed into a v1 command.
const DeviceProtocolVersion = 1
