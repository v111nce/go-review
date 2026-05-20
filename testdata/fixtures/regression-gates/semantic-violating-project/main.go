package main

import "os"

func Message() string {
	return os.Getenv("MESSAGE")
}
