package auth

import (
    "crypto/rand"
    "encoding/hex"
)


// 生成随机 Session ID
func NewSessionID() (string, error) {
    var id [32]byte
    if _, err := rand.Read(id[:]); err != nil {
        return "", err
    }
	
    return hex.EncodeToString(id[:]), nil
}