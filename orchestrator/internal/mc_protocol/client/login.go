// Package client builds packets sent by a Minecraft protocol client.
// Reference documentation for login can be found at: https://minecraft.wiki/w/Java_Edition_protocol/FAQ#What's_the_normal_login_sequence_for_a_client?
package client

import (
	"crypto/md5"
	"fmt"
	"math/rand"
	"time"
)

const loginState = 2

func offlineUUID(username string) [16]byte {
	uuid := md5.Sum([]byte("OfflinePlayer:" + username))
	uuid[6] = uuid[6]&0x0f | 0x30
	uuid[8] = uuid[8]&0x3f | 0x80
	return uuid
}

func GenRandomUserName() string {
	prefixList := []string {
		"King",
		"Star",
		"Card",
		"Platinum",
		"John",
		"Monk",
		"Zen",
	}
	surfixList := []string {
		"Steve",
		"Doom",
		"Ram",
		"Robot",
		"Ansible",
	}

	r := rand.New(rand.NewSource(time.Now().UnixNano()))

	prefix := prefixList[r.Intn(len(prefixList))]
	surfix := surfixList[r.Intn(len(surfixList))]
	ranNum := r.Intn(1000) + 100

	return fmt.Sprintf("%s%s%d", prefix, surfix, ranNum)
	
}