/*
 * Copyright (c) 2022. TxnLab Inc.
 * All Rights reserved.
 */
package misc

import (
	"os"
)

var secretsMap = map[string]string{}

func GetSecret(key string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return secretsMap[key]
}
