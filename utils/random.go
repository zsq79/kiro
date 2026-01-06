package utils

import (
	"crypto/rand"
	"encoding/hex"
	"math/big"
	mathrand "math/rand"
	"sync"
	"time"
)

var (
	fallbackRand     = mathrand.New(mathrand.NewSource(time.Now().UnixNano()))
	fallbackRandLock sync.Mutex
)

func cryptoInt(max int64) (int64, error) {
	if max <= 0 {
		return 0, nil
	}
	n, err := rand.Int(rand.Reader, big.NewInt(max))
	if err != nil {
		return 0, err
	}
	return n.Int64(), nil
}

func RandomIntBetween(min, max int64) int64 {
	if min >= max {
		return min
	}
	span := max - min + 1
	n, err := cryptoInt(span)
	if err != nil {
		fallbackRandLock.Lock()
		defer fallbackRandLock.Unlock()
		return min + fallbackRand.Int63n(span)
	}
	return min + n
}

func RandomBool() bool {
	return RandomIntBetween(0, 1) == 1
}

func RandomHex(length int) string {
	if length <= 0 {
		return ""
	}
	bytes := make([]byte, (length+1)/2)
	if _, err := rand.Read(bytes); err != nil {
		fallbackRandLock.Lock()
		fallbackRand.Read(bytes)
		fallbackRandLock.Unlock()
	}
	hexString := hex.EncodeToString(bytes)
	return hexString[:length]
}

func ShuffleStrings(values []string) {
	if len(values) <= 1 {
		return
	}
	for i := len(values) - 1; i > 0; i-- {
		j := int(RandomIntBetween(0, int64(i)))
		values[i], values[j] = values[j], values[i]
	}
}

func ShuffleUint16(values []uint16) {
	if len(values) <= 1 {
		return
	}
	for i := len(values) - 1; i > 0; i-- {
		j := int(RandomIntBetween(0, int64(i)))
		values[i], values[j] = values[j], values[i]
	}
}
