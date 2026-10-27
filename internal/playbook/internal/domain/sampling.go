package domain

import (
	"crypto/sha256"
	"encoding/binary"
	"strconv"
)

func AuditDue(situationID string, version, every int) bool {
	if every <= 1 {
		return true
	}
	sum := sha256.Sum256([]byte(situationID + "\x00" + strconv.Itoa(version)))
	return binary.BigEndian.Uint64(sum[:8])%uint64(every) == 0
}
