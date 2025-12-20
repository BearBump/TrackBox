package main

import (
	"context"
)

func main() {
	app := mustBootstrapTrackAPI()
	defer app.Close()

	if err := app.Run(); err != nil && err != context.Canceled {
		panic(err)
	}
}


