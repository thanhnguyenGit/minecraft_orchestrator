// Package client builds packets sent by a Minecraft protocol client.
package client

import "crypto/md5"

const loginState = 2

func offlineUUID(username string) [16]byte {
	uuid := md5.Sum([]byte("OfflinePlayer:" + username))
	uuid[6] = uuid[6]&0x0f | 0x30
	uuid[8] = uuid[8]&0x3f | 0x80
	return uuid
}
