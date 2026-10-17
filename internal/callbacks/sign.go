package callbacks

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"time"
)


func Sign(secret string, now time.Time, body []byte) string {
	timestamp := strconv.FormatInt(now.Unix(), 10)

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestamp))
	mac.Write([]byte("."))
	mac.Write(body)

	return fmt.Sprintf("t=%s,v1=%s", timestamp, hex.EncodeToString(mac.Sum(nil)))
}
