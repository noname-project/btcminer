package main

import (
	"fmt"
	"os"
	"runtime"

	"github.com/boomstarternetwork/btcminer/stratum"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli"
)

func main() {
	runtime.GOMAXPROCS(runtime.NumCPU())

	var minersCount uint
	switch runtime.NumCPU() {
	case 1, 2:
		minersCount = uint(runtime.NumCPU())
	default:
		minersCount = uint(runtime.NumCPU() - 1)
	}

	maxMinersCount := runtime.NumCPU()

	app := cli.NewApp()
	app.Name = "btcminer"
	app.Usage = ""
	app.Description = "Bitcoin like coins stratum miner."
	app.Author = "Vadim Chernov"
	app.Email = "v.chernov@boomstarter.ru"
	app.Version = "0.1"

	app.Action = miner

	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "pool-address, pa",
			Usage: "pool address, e.g example.com:3000",
		},
		cli.StringFlag{
			Name: "login, l",
			Usage: "your pool login, e.g 2N8RotpoEiG934JSywRdPodCcZj9aMTrmBE." +
				"worker",
		},
		cli.StringFlag{
			Name:  "password, p",
			Usage: "your pool password",
			Value: "",
		},
		cli.StringFlag{
			Name:  "algorithm, a",
			Usage: "mining algorithm, one of: sha256d, scrypt.",
		},
		cli.UintFlag{
			Name: "miners-count, mc",
			Usage: fmt.Sprintf("miners threads count, 1 <= count <= %d",
				maxMinersCount),
			Value: minersCount,
		},
		cli.StringFlag{
			Name: "verbosity, vs",
			Usage: "logger verbosity level, one of: debug, info, warn, error" +
				", fatal, panic",
		},
	}

	err := app.Run(os.Args)
	if err != nil {
		fmt.Println(err.Error())
	}
}

func miner(c *cli.Context) error {
	poolAddress := c.String("pool-address")
	login := c.String("login")
	password := c.String("password")
	algorithmStr := c.String("algorithm")
	minersCount := c.Uint("miners-count")
	verbosity := c.String("verbosity")

	algorithm, err := stratum.ParseAlgorithm(algorithmStr)
	if err != nil {
		return cli.NewExitError("failed to parse algorithm: "+err.Error(), 1)
	}

	if minersCount < 1 || minersCount > uint(runtime.NumCPU()) {
		return cli.NewExitError("invalid miners count", 2)
	}

	logrusLevel, err := logrus.ParseLevel(verbosity)
	if err != nil {
		return cli.NewExitError("failed to parse verbosity: "+err.Error(), 3)
	}

	logrus.SetLevel(logrusLevel)
	logrus.SetFormatter(&logrus.TextFormatter{
		FullTimestamp:   true,
		TimestampFormat: "2006-01-02 15:04:05",
	})

	sc := stratum.NewClient(stratum.ClientParams{
		PoolAddress: poolAddress,
		Login:       login,
		Password:    password,
		Algorithm:   algorithm,
		MinersCount: minersCount,
	})

	err = sc.Serve()
	if err != nil {
		return cli.NewExitError("failed to start stratum client: "+
			err.Error(), 4)
	}

	return nil
}
