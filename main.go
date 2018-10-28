package main

import (
	"os"
	"runtime"

	"github.com/boomstarternetwork/btcminer/stratum"
	"github.com/sirupsen/logrus"
)

func main() {
	logrus.SetLevel(logrus.DebugLevel)
	logrus.SetFormatter(&logrus.TextFormatter{
		FullTimestamp:   true,
		TimestampFormat: "2006-01-02 15:04:05",
	})

	runtime.GOMAXPROCS(runtime.NumCPU())

	var minersCount uint
	switch runtime.NumCPU() {
	case 1, 2:
		minersCount = uint(runtime.NumCPU())
	default:
		minersCount = uint(runtime.NumCPU() - 1)
	}

	c := stratum.NewClient(stratum.ClientParams{
		URL:         "127.0.0.1:3000",
		Login:       "2N8RotpoEiG934JSywRdPodCcZj9aMTrmBE.horns-and-hooves",
		Password:    "",
		Algorithm:   stratum.SHA256d,
		MinersCount: minersCount,
	})

	err := c.Serve()
	if err != nil {
		logrus.WithError(err).Error("Failed to start stratum client")
		os.Exit(1)
	}
}
