package billing

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync/atomic"
	"time"
)

var idFallbackCounter atomic.Uint64

func newID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err == nil {
		return hex.EncodeToString(b)
	}

	// Extremely unlikely fallback when crypto/rand is unavailable.
	n := idFallbackCounter.Add(1)
	return fmt.Sprintf("%x%x", time.Now().UTC().UnixNano(), n)
}

func newSubID() string {
	return newID()
}

func newReservationID() string {
	return newID()
}
