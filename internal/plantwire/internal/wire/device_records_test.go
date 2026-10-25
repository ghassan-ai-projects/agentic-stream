package wire_test

import (
	"encoding/json"
	"testing"
)

func TestDeviceRecordsArriveWithTheirBodies(t *testing.T) {
	t.Parallel()
	birth := arrivalOf(t, fixture(t, "DBIRTH.json")).DeviceBirth
	if birth == nil || birth.Device != "P1" || birth.Kind != "pump" || birth.BootID != "board-01-boot-1" ||
		birth.BirthTimeNS != 1767225600000000000 || len(birth.Metrics) != 3 || birth.Metrics[1].Name != "speed_setpoint" || birth.Metrics[1].Alias != 2 {
		t.Errorf("DBIRTH body = %+v", birth)
	}
	data := arrivalOf(t, fixture(t, "DDATA.json")).DeviceData
	if data == nil || data.Device != "P1" || data.Historical || len(data.Metrics) != 1 || data.Metrics[0].Value != json.Number("42.0") ||
		data.Metrics[0].EventTimeNS != 1767225660000000000 || data.Metrics[0].Quality != "good" || data.Metrics[0].DeviceMonoUS != nil {
		t.Errorf("DDATA body = %+v", data)
	}
	if death := arrivalOf(t, fixture(t, "DDEATH.json")).DeviceDeath; death == nil || death.Device != "P1" || death.BootID != "board-01-boot-1" {
		t.Errorf("DDEATH body = %+v", death)
	}
	if barrier := arrivalOf(t, fixture(t, "NSYNC.json")).Sync; barrier == nil || barrier.WorldTimeNS != 1767225660000000000 {
		t.Errorf("NSYNC body = %+v", barrier)
	}
}
