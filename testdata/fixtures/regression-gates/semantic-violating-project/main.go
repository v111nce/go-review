package fixture

import "os"

func Message() string {
	return os.Getenv("MESSAGE")
}
