package stratum

import (
	"errors"
	"net"
	"time"

	"github.com/boomstarternetwork/btcminer/miner"
	"github.com/boomstarternetwork/stratum"
	"github.com/sirupsen/logrus"
)

// Client is stratum miner client.
type Client struct {
	poolAddress string
	login       string
	password    string
	algorithm   miner.Algorithm
	minersCount uint

	latestMinerParams miner.Params

	client *stratum.BitcoinClient

	subscription *subscription
}

// ClientParams is a params required to start stratum miner client.
type ClientParams struct {
	PoolAddress string
	Login       string
	Password    string
	Algorithm   miner.Algorithm
	MinersCount uint
}

// NewClient creates new stratum client.
func NewClient(p ClientParams) *Client {
	return &Client{
		poolAddress: p.PoolAddress,
		login:       p.Login,
		password:    p.Password,
		algorithm:   p.Algorithm,
		minersCount: p.MinersCount,
		subscription: &subscription{
			minersCount: p.MinersCount,
		},
	}
}

const (
	agent = "btcminer/0.1"

	errCodeJobNotFound = 21
)

// Serve starts mining.
func (c *Client) Serve() error {
	conn, err := net.Dial("tcp", c.poolAddress)
	if err != nil {
		return err
	}

	c.client = stratum.NewBitcoinClient(conn, c)

	logrus.Debug("Authorizing...")

	authorized, err := c.client.Authorize(&stratum.LoginParams{
		User:     c.login,
		Password: c.password,
	})
	if err != nil {
		return errors.New("failed to authorize: " + err.Error())
	}
	if !authorized {
		return errors.New("failed to authorize: pool returned false")
	}

	logrus.Debug("Authorized")

	logrus.Debug("Subscribing...")

	res, err := c.client.Subscribe(&stratum.SubscribeBitcoinParams{
		Agent:      agent,
		ExtraNonce: "-",
	})
	if err != nil {
		return errors.New("failed to subscribe: " + err.Error())
	}

	c.subscription.set(res.Subscriptions[0].ID, res.ExtraNonce,
		uint(res.ExtraNonceSize))

	logrus.Debug("Subscribed")

	for {
		time.Sleep(1 * time.Hour)
	}
}

func (c *Client) OnReconnect(params *stratum.ReconnectParams) {
	logrus.WithField("params", params).Debug("Reconnect server call")
}

func (c *Client) OnShowMessage(msg string) {
	logrus.WithField("msg", msg).Debug("Show message server call")
}

func (c *Client) OnSetDifficulty(difficulty float64) {
	logrus.WithField("difficulty",
		difficulty).Debug("Set difficulty server call")

	c.subscription.setDifficulty(difficulty)
}

func (c *Client) OnSetExtraNonce(params *stratum.ExtraNonceParams) {
	logrus.WithField("params", params).Debug("Set extra nonce server call")
}

func (c *Client) OnNotify(params *stratum.NotifyBitcoinData) {
	logrus.WithField("params", params).Debug("Notify server call")

	mp := miner.Params{
		JobID:          params.JobID,
		PrevHash:       params.PrevHash,
		Coinb1:         params.CoinBasePart1,
		Coinb2:         params.CoinBasePart2,
		MerkleBranches: params.MerkleBranch,
		Version:        params.Version,
		Nbits:          params.NBits,
		Ntime:          params.NTime,
		Algorithm:      c.algorithm,
		MinersCount:    c.minersCount,
	}

	c.latestMinerParams = mp

	if params.CleanJobs || c.subscription.noMiner() {
		c.startMiner(mp)
	}
}

func (c *Client) startMiner(mp miner.Params) {
	logrus.Debug("Starting miner...")

	shares, err := c.subscription.newMiner(mp)
	if err != nil {
		logrus.WithError(err).Error("Failed to create new miner")
	}

	logrus.Debug("Miner started")

	go c.handleShares(shares)
}

func (c *Client) handleShares(shares chan miner.Share) {
	s := <-shares

	logrus.WithField("share", s).Info("Found share, submitting...")

	submitted, err := c.client.Submit(&stratum.SubmitBitcoinParams{
		User:       c.login,
		JobID:      s.JobID,
		ExtraNonce: s.ExtraNonce2,
		NTime:      s.Ntime,
		NOnce:      s.Nonce,
	})

	if submitted {
		logrus.Info("Share submitted")
	} else if err == nil {
		logrus.Info("Share not submitted")
	} else {
		logrus.WithError(err).Error("Failed to submit share")

		if resErr, ok := err.(*stratum.ResponseError); ok {
			if resErr.Code() == errCodeJobNotFound {
				// We need to start new job with latest params.
				c.startMiner(c.latestMinerParams)
				return
			}
		}

		// By default we just continue to mine current job.
		logrus.Info("Continue to mine...")
		shares := c.subscription.continueMine()
		go c.handleShares(shares)
	}
}
