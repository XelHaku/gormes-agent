// gormes/internal/subagent/ids.go
package subagent

import (
	"crypto/rand"
	"encoding/base32"
	"fmt"
)

// newSubagentID returns a fresh subagent ID of the form "sa_<13-char-base32>".
// 8 bytes of crypto/rand entropy → 13 base32 (no-padding) characters, giving
// 64 bits of randomness — collision-resistant for any realistic subagent volume.
func newSubagentID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand.Read is documented as never returning short reads on
		// supported platforms. A failure here means the OS RNG is broken;
		// continuing would silently undermine ID uniqueness.
		panic(fmt.Errorf("subagent: crypto/rand failed: %w", err))
	}
	return "sa_" + base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b[:])
}
