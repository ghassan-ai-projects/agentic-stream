# Event catalog reference

The built-in event registry is data-driven and loaded from
[`internal/eventschema/registry_data.json`](../../internal/eventschema/registry_data.json).
The current catalog includes these families:

| Family | Examples |
| --- | --- |
| Motor | temperature, vibration, current, heartbeat |
| Pump | temperature, vibration, axial vibration, current, RPM, flow, pressure, turbidity, tank level, mode, demand, heartbeat |
| Reefer | ambient/return/supply temperature, setpoint, defrost, door, link, power |
| Pond | dissolved oxygen, ammonia, pH, water temperature, aerator current, feeding, heartbeat |
| Bay | air temperature, humidity, CO₂, PAR light, leaf wetness, vent position/event, heartbeat |
| Generic fixture | sensor temperature |

Entries use versioned event types such as
`motor.vibration.observed/1.0` and declare payload fields, units, bounds, and
classification expectations. The JSON file is the authority; this table is a
navigation summary, not a replacement catalog.

## Simulator mappings

The simulator channel-to-field mapping is in
[`internal/ingress/simulator_data.json`](../../internal/ingress/simulator_data.json).
Use `--trace-format simulator` only for the documented simulator record shape.

## Adding an event

Update data, add registry/conversion tests, review digest changes, and update
the public guide if the event becomes a supported example. Never add the
schema as a Go literal.

## Next reads

- [Event envelope](../contracts/event-envelope.md)
- [Add a domain](../guides/add-a-domain.md)
- [First SituationSpec](../getting-started/first-situation.md)
