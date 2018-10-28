package stratum

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// Client is stratum miner client.
type Client struct {
	url         string
	login       string
	password    string
	algorithm   Algorithm
	minersCount uint

	connection net.Conn
	messageID  uint

	requests        map[uint]request
	latestJobParams jobParams

	subscription *subscription

	mutex sync.RWMutex
}

type ClientParams struct {
	URL         string
	Login       string
	Password    string
	Algorithm   Algorithm
	MinersCount uint
}

// NewClient creates new stratum client.
func NewClient(p ClientParams) *Client {
	return &Client{
		url:         p.URL,
		login:       p.Login,
		password:    p.Password,
		algorithm:   p.Algorithm,
		minersCount: p.MinersCount,
		requests:    map[uint]request{},
		subscription: &subscription{
			minersCount: p.MinersCount,
		},
	}
}

func (c *Client) request(messageID uint) (request, bool) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	req, exists := c.requests[messageID]
	return req, exists
}

func (c *Client) storeRequest(req request) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.requests[req.ID] = req
}

func (c *Client) removeRequest(messageID uint) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	delete(c.requests, messageID)
}

// call calls pool JSON RPC method with given params. It registers
// request in requests map under current messageID to handle answer
// later.
func (c *Client) call(method string, params ...interface{}) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	logrus.WithFields(logrus.Fields{
		"method": method,
		"params": params,
	}).Info("Calling JSON RPC")

	if c.connection == nil {
		return errors.New("connection is nil")
	}

	req := request{
		ID:     c.messageID,
		Method: method,
		Params: params,
	}

	reqJSON, err := json.Marshal(req)
	if err != nil {
		return err
	}

	reqJSON = append(reqJSON, '\n')

	writtenBytes := 0
	for writtenBytes != len(reqJSON) {
		n, err := c.connection.Write(reqJSON[writtenBytes:])
		if err != nil {
			return err
		}
		writtenBytes += n
	}

	c.requests[c.messageID] = req

	c.messageID++

	return nil
}

// unmarshalJSONLine unmarshal JSON RPC line, checks if it request or response
// and gets corresponding request if line is response.
func (c *Client) unmarshalJSONLine(JSONLine []byte) (request, response, error) {
	var (
		req request
		res response
	)

	err := json.Unmarshal(JSONLine, &req)
	if err != nil || req.Method == "" {

		err := json.Unmarshal(JSONLine, &res)
		if err != nil {
			return req, res,
				errors.New("failed to unmarshal JSON line to neither" +
					" request nor response")
		}

		var exists bool

		req, exists = c.request(res.ID)
		if !exists {
			return req, res,
				errors.New("can't find response's corresponding request")
		}

		c.removeRequest(res.ID)
	}

	return req, res, nil
}

// handleIncomingJSONLines handles connection's incoming JSON lines.
func (c *Client) handleIncomingJSONLines() (err error) {
	r := bufio.NewReader(c.connection)
	for {
		var (
			isPrefix = true
			JSONLine []byte
		)
		for isPrefix {
			var line []byte
			line, isPrefix, err = r.ReadLine()
			if err != nil {
				return err
			}
			JSONLine = append(JSONLine, line...)
		}

		//logrus.WithField("JSONLine", string(JSONLine)).
		//	Debug("New JSON RPC line")

		req, res, err := c.unmarshalJSONLine(JSONLine)
		if err != nil {
			logrus.WithError(err).Error(
				"Failed to unmarshal JSON RPC line")
		}

		err = c.handleRPCCall(req, res)
		if err != nil {
			logrus.WithError(err).Error(
				"Error occurred during handing JSON RPC call")
		}
	}
	return nil
}

const (
	methodSubscribe     = "mining.subscribe"
	methodAuthorize     = "mining.authorize"
	methodNotify        = "mining.notify"
	methodSetDifficulty = "mining.set_difficulty"
	methodSubmit        = "mining.submit"

	errCodeJobNotFound = 21
)

func (c *Client) handleRPCCall(req request, res response) error {

	logrus.WithFields(logrus.Fields{
		"req": req,
		"res": res,
	}).Info("Handling RPC call")

	switch req.Method {
	case methodAuthorize:
		// this is outgoing request, we need to handle response
		if res.Error != nil {
			return errors.New("response error: " + res.Error.Message)
		}
		if !res.Result.(bool) {
			return fmt.Errorf("failed to authorize")
		}

		err := c.call(methodSubscribe, "btcminer/0.1")
		if err != nil {
			return err
		}

	case methodSubscribe:
		// this is outgoing request, we need to handle response
		if res.Error != nil {
			return errors.New("response error: " + res.Error.Message)
		}
		result, ok := res.Result.([]interface{})
		if !ok {
			return errors.New("failed to cast response result")
		}

		if len(result) != 3 {
			return errors.New("expected response result array length is 3")
		}

		subscriptionIDArray, ok := result[0].([]interface{})
		if !ok {
			return errors.New("failed to cast subscription ID array")
		}

		subscriptionIDArray, ok = subscriptionIDArray[0].([]interface{})
		if !ok {
			return errors.New("failed to cast subscription ID array")
		}

		subscriptionID, ok := subscriptionIDArray[1].(string)
		if !ok {
			return errors.New("failed to cast subscription ID")
		}

		extraNonce1, ok := result[1].(string)
		if !ok {
			return errors.New("failed to cast extraNonce1")
		}

		extraNonce2Length, ok := result[2].(float64)
		if !ok {
			return errors.New("failed to cast extraNonce2Length")
		}

		c.subscription.set(subscriptionID, extraNonce1,
			uint(extraNonce2Length))

	case methodSetDifficulty:
		// this is incoming request, we need to handle request
		if res.Error != nil {
			return errors.New("response error: " + res.Error.Message)
		}
		if len(req.Params) != 1 {
			return fmt.Errorf("expected request params length is 1")
		}

		difficulty, ok := req.Params[0].(float64)
		if !ok {
			return errors.New("failed to cast difficulty")
		}

		c.subscription.setDifficulty(difficulty)

	case methodNotify:
		// this is incoming request, we need to handle request
		if res.Error != nil {
			return errors.New("response error: " + res.Error.Message)
		}
		if len(req.Params) != 9 {
			return fmt.Errorf("expected request params length is 9")
		}

		var ok bool
		var jp jobParams

		jp.jobID, ok = req.Params[0].(string)
		if !ok {
			return errors.New("failed to cast jobID")
		}

		jp.prevHash, ok = req.Params[1].(string)
		if !ok {
			return errors.New("failed to cast prevHash")
		}

		jp.coinb1, ok = req.Params[2].(string)
		if !ok {
			return errors.New("failed to cast coinb1")
		}

		jp.coinb2, ok = req.Params[3].(string)
		if !ok {
			return errors.New("failed to cast coinb2")
		}

		merkleBranches, ok := req.Params[4].([]interface{})
		if !ok {
			return errors.New("failed to cast merkleBranches")
		}

		for _, mb := range merkleBranches {
			mbStr, ok := mb.(string)
			if !ok {
				return errors.New("failed to cast merkle branch")
			}
			jp.merkleBranches = append(jp.merkleBranches, mbStr)
		}

		jp.version, ok = req.Params[5].(string)
		if !ok {
			return errors.New("failed to cast version")
		}

		jp.nbits, ok = req.Params[6].(string)
		if !ok {
			return errors.New("failed to cast nbits")
		}

		jp.ntime, ok = req.Params[7].(string)
		if !ok {
			return errors.New("failed to cast ntime")
		}

		jp.hashFunc = c.algorithm.hashFunc()
		jp.minersCount = c.minersCount

		c.latestJobParams = jp

		cleanJobs, ok := req.Params[8].(bool)
		if !ok {
			return errors.New("failed to cast cleanJobs")
		}

		if cleanJobs || c.subscription.noJob() {
			shares, err := c.subscription.newJob(jp)
			if err != nil {
				return err
			}

			go func() {
				s := <-shares
				c.call(methodSubmit, c.login, s.JobID, s.ExtraNonce2,
					s.Ntime, s.Nonce)
			}()
		}

	case methodSubmit:
		// this is outgoing request, we need to handle response
		if res.Error != nil {
			logrus.WithFields(logrus.Fields{
				"errCode": res.Error.Code,
				"err":     res.Error.Message,
			}).Error("Failed to submit share")

			switch res.Error.Code {
			case errCodeJobNotFound:
				// We need to start new job with latest params.
				shares, err := c.subscription.newJob(c.latestJobParams)
				if err != nil {
					return err
				}

				go func() {
					s := <-shares
					c.call(methodSubmit, c.login, s.JobID, s.ExtraNonce2,
						s.Ntime, s.Nonce)
				}()

			default:
				// By default we just continue to mine current job.
				shares := c.subscription.continueJob()
				go func() {
					s := <-shares
					c.call(methodSubmit, c.login, s.JobID, s.ExtraNonce2,
						s.Ntime, s.Nonce)
				}()
			}
		}

	default:
		if req.Method != "" {
			logrus.WithField("method", req.Method).
				Warn("Unsupported method call")
		}
	}
	return nil
}

func (c *Client) Serve() error {
	conn, err := net.Dial("tcp", c.url)
	if err != nil {
		return err
	}

	c.connection = conn

	go c.handleIncomingJSONLines()

	err = c.call(methodAuthorize, c.login, c.password)
	if err != nil {
		return err
	}

	for {
		time.Sleep(1 * time.Hour)
	}
}

type request struct {
	ID     uint          `json:"id"`
	Method string        `json:"method"`
	Params []interface{} `json:"params"`
}

type response struct {
	ID     uint        `json:"id"`
	Result interface{} `json:"result"`
	Error  *struct {
		Code    uint   `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}
