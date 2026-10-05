package domain

// Owner is the runtime process that holds the singleton runtime lease: one
// ownership period (Epoch) of one process (Instance).
type Owner struct {
	Epoch    string
	Instance string
}

// Complete reports whether both owner identities are present.
func (o Owner) Complete() bool {
	return o.Epoch != "" && o.Instance != ""
}

// DeviceBoot is one power-on of one device. A reboot is a new device boot.
type DeviceBoot struct {
	DeviceID string
	BootID   string
}

// Complete reports whether both device identities are present.
func (d DeviceBoot) Complete() bool {
	return d.DeviceID != "" && d.BootID != ""
}

// TargetClaim is an owner's right to command one target on one device boot.
// It also names the subject of an authority event.
type TargetClaim struct {
	Target string
	Device DeviceBoot
	Owner  Owner
}

// Complete reports whether every identity of the claim is present.
func (c TargetClaim) Complete() bool {
	return c.Target != "" && c.Device.Complete() && c.Owner.Complete()
}

// DeviceSubject names an authority-event subject that concerns the whole
// device rather than one output: its target is the device ID.
func DeviceSubject(device DeviceBoot, owner Owner) TargetClaim {
	return TargetClaim{Target: device.DeviceID, Device: device, Owner: owner}
}
