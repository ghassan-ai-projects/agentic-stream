// Package app holds the device-authority use cases. Each one validates its
// input, opens one store unit of work (admitted, or on the priority path),
// loads state, decides with the domain rules, persists and audits. It knows
// no SQL and no transaction handles: persistence goes through store.Tx.
package app
