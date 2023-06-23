package main

import (
	"context"

	"github.com/can3p/anti-disposable-email/update"
)

func main() {
	err := update.Update(context.Background(), "list.go")

	if err != nil {
		panic(err)
	}
}
